package grpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"runtime"
	"sync"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type OutsideHandler func(ctx *Context)

type ServerConfig struct {
	//global timeout for every rpc call(including connection establish time)
	GlobalTimeout time.Duration
	HeartPorbe    time.Duration
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	CertKeys      map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe < time.Second {
		c.HeartPorbe = 1500 * time.Millisecond
	}
	if c.SocketRBuf == 0 {
		c.SocketRBuf = 1024
	}
	if c.SocketRBuf > 65535 {
		c.SocketRBuf = 65535
	}
	if c.SocketWBuf == 0 {
		c.SocketWBuf = 1024
	}
	if c.SocketWBuf > 65535 {
		c.SocketWBuf = 65535
	}
	if c.MaxMsgLen < 1024 {
		c.MaxMsgLen = 65535
	}
	if c.MaxMsgLen > 65535 {
		c.MaxMsgLen = 65535
	}
}

type GrpcServer struct {
	c           *ServerConfig
	selfappname string
	global      []OutsideHandler
	ctxpool     *sync.Pool
	server      *grpc.Server
	services    map[string]*grpc.ServiceDesc
}

func NewGrpcServer(c *ServerConfig, selfgroup, selfname string) (*GrpcServer, error) {
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return nil, e
	}
	appname := selfgroup + "." + selfname
	if e := common.NameCheck(appname, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	serverinstance := &GrpcServer{
		c:           c,
		selfappname: selfgroup + "." + selfname,
		global:      make([]OutsideHandler, 0),
		ctxpool:     &sync.Pool{},
		services:    make(map[string]*grpc.ServiceDesc),
	}
	c.validate()
	opts := make([]grpc.ServerOption, 0, 6)
	opts = append(opts, grpc.ReadBufferSize(int(c.SocketRBuf)))
	opts = append(opts, grpc.WriteBufferSize(int(c.SocketWBuf)))
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.MaxSendMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.ConnectionTimeout(c.GlobalTimeout))
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{Time: c.HeartPorbe, Timeout: c.HeartPorbe*3 + c.HeartPorbe/3}))
	if len(c.CertKeys) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.CertKeys))
		for cert, key := range c.CertKeys {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[crpc.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: certificates})))
	}
	serverinstance.server = grpc.NewServer(opts...)
	return serverinstance, nil
}

var ErrServerClosed = errors.New("[grpc.server] closed")

func (s *GrpcServer) StartGrpcServer(listenaddr string) error {
	l, e := net.Listen("tcp", listenaddr)
	if e != nil {
		return errors.New("[grpc.server] listen tcp addr: " + listenaddr + " error:" + e.Error())
	}
	for _, service := range s.services {
		s.server.RegisterService(service, nil)
	}
	if e := s.server.Serve(l); e != nil {
		if e == grpc.ErrServerStopped {
			return ErrServerClosed
		}
		return e
	}
	return nil
}
func (s *GrpcServer) StopGrpcServer() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

//thread unsafe
func (s *GrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
func (s *GrpcServer) RegisterHandler(sname, mname string, functimeout time.Duration, handlers ...OutsideHandler) {
	service, ok := s.services[sname]
	if !ok {
		service = &grpc.ServiceDesc{
			ServiceName: sname,
			HandlerType: nil,
			Methods:     []grpc.MethodDesc{},
			Streams:     []grpc.StreamDesc{},
			Metadata:    nil,
		}
		s.services[sname] = service
	}
	service.Methods = append(service.Methods, grpc.MethodDesc{
		MethodName: mname,
		Handler:    s.insidehandler(sname, mname, functimeout, handlers...),
	})
}
func (s *GrpcServer) insidehandler(sname, mname string, functimeout time.Duration, handlers ...OutsideHandler) func(interface{}, context.Context, func(interface{}) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := append(s.global, handlers...)
	return func(_ interface{}, ctx context.Context, decode func(interface{}) error, _ grpc.UnaryServerInterceptor) (resp interface{}, e error) {
		p, _ := peer.FromContext(ctx)
		traceid := ""
		sourceip := p.Addr.String()
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		grpcmetadata, ok := metadata.FromIncomingContext(ctx)
		if ok {
			data := grpcmetadata.Get("core_tracedata")
			if len(data) != 4 && len(data) != 0 {
				log.Error(nil, "[grpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC error: tracedata format error")
				return nil, cerror.ErrReq
			}
			if len(data) > 0 {
				traceid = data[0]
				sourceapp = data[1]
				sourcemethod = data[2]
				sourcepath = data[3]
			}
		}
		ctx = trace.InitTrace(ctx, traceid, s.selfappname, host.Hostip, "GRPC", path)
		traceid, _, _, _, _ = trace.GetTrace(ctx)
		var mdata map[string]string
		if ok {
			data := grpcmetadata.Get("core_metadata")
			if len(data) != 0 {
				mdata = make(map[string]string)
				if e := json.Unmarshal(common.Str2byte(data[0]), &mdata); e != nil {
					log.Error(nil, "[grpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC metadata:", data[0], "format error:", e)
					return nil, cerror.ErrReq
				}
			}
		}
		var min time.Duration
		if s.c.GlobalTimeout != 0 && functimeout != 0 {
			if s.c.GlobalTimeout < functimeout {
				min = s.c.GlobalTimeout
			} else {
				min = functimeout
			}
		} else if s.c.GlobalTimeout != 0 {
			min = s.c.GlobalTimeout
		} else {
			min = functimeout
		}
		if min != 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, min)
			defer cancel()
		}
		start := time.Now()
		if dl, ok := ctx.Deadline(); ok && dl.UnixNano() < start.UnixNano()+int64(time.Millisecond) {
			resp = nil
			e = cerror.ErrDeadlineExceeded
			return
		}
		workctx := s.getcontext(ctx, path, sourceapp, sourceip, mdata, totalhandlers, decode)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[grpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				workctx.e = ErrPanic
				workctx.resp = nil
			}
			end := time.Now()
			trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath), trace.SERVER, s.selfappname, host.Hostip, "GRPC", path, &start, &end, workctx.e)
			resp = workctx.resp
			if workctx.e != nil {
				e = workctx.e
			}
		}()
		workctx.run()
		return
	}
}
