package cgrpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	ConnectTimeout time.Duration
	GlobalTimeout  time.Duration //global timeout for every rpc call(including connection establish time)
	HeartPorbe     time.Duration
	SocketRBuf     uint32
	SocketWBuf     uint32
	MaxMsgLen      uint32
	CertKeys       map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
	}
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

type CGrpcServer struct {
	c              *ServerConfig
	selfappname    string //group.name
	global         []OutsideHandler
	ctxpool        *sync.Pool
	server         *grpc.Server
	clientnum      int32
	services       map[string]*grpc.ServiceDesc
	handlerTimeout map[string]time.Duration

	totalreqnum int32
}

func NewCGrpcServer(c *ServerConfig, selfgroup, selfname string) (*CGrpcServer, error) {
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:              c,
		selfappname:    selfappname,
		global:         make([]OutsideHandler, 0),
		ctxpool:        &sync.Pool{},
		services:       make(map[string]*grpc.ServiceDesc),
		handlerTimeout: make(map[string]time.Duration),
	}
	opts := make([]grpc.ServerOption, 0, 6)
	opts = append(opts, grpc.StatsHandler(serverinstance))
	opts = append(opts, grpc.ReadBufferSize(int(c.SocketRBuf)))
	opts = append(opts, grpc.WriteBufferSize(int(c.SocketWBuf)))
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.MaxSendMsgSize(int(c.MaxMsgLen)))
	if c.ConnectTimeout != 0 {
		opts = append(opts, grpc.ConnectionTimeout(c.ConnectTimeout))
	}
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

var ErrServerClosed = errors.New("[cgrpc.server] closed")

func (s *CGrpcServer) StartCGrpcServer(listenaddr string) error {
	l, e := net.Listen("tcp", listenaddr)
	if e != nil {
		return errors.New("[cgrpc.server] listen tcp addr: " + listenaddr + " error:" + e.Error())
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
func (s *CGrpcServer) StopCGrpcServer() {
	s.server.GracefulStop()
}

func (this *CGrpcServer) GetClientNum() int32 {
	return atomic.LoadInt32(&this.clientnum)
}
func (this *CGrpcServer) GetReqNum() int32 {
	return atomic.LoadInt32(&this.totalreqnum)
}

//map key path,map value handler timeout,0 means no handler specific timeout,but still has global timeout
func (this *CGrpcServer) UpdateHandlerTimeout(htcs map[string]time.Duration) {
	tmp := make(map[string]time.Duration)
	for path, timeout := range htcs {
		if timeout <= 0 {
			//jump,0 means no handler specific timeout
			continue
		}
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}
		tmp[path] = timeout
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout)), unsafe.Pointer(&tmp))
}

func (this *CGrpcServer) getHandlerTimeout(path string) time.Duration {
	handlerTimeout := *(*map[string]time.Duration)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout))))
	if t, ok := handlerTimeout[path]; ok && t != 0 {
		return t
	}
	return this.c.GlobalTimeout
}

//thread unsafe
func (s *CGrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
func (s *CGrpcServer) RegisterHandler(sname, mname string, handlers ...OutsideHandler) {
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
		Handler:    s.insidehandler(sname, mname, handlers...),
	})
}
func (s *CGrpcServer) insidehandler(sname, mname string, handlers ...OutsideHandler) func(interface{}, context.Context, func(interface{}) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ interface{}, ctx context.Context, decode func(interface{}) error, _ grpc.UnaryServerInterceptor) (resp interface{}, e error) {
		grpcmetadata, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if data := grpcmetadata.Get("core_target"); len(data) != 0 && data[0] != s.selfappname {
				return nil, cerror.ErrClosing
			}
		}
		atomic.AddInt32(&s.totalreqnum, 1)
		defer atomic.AddInt32(&s.totalreqnum, -1)
		p, _ := peer.FromContext(ctx)
		traceid := ""
		sourceip := p.Addr.String()
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		selfdeep := 0
		if ok {
			if data := grpcmetadata.Get("core_tracedata"); len(data) == 0 || data[0] == "" {
				ctx = trace.InitTrace(ctx, "", s.selfappname, host.Hostip, "GRPC", path, 0)
			} else if len(data) != 5 || data[4] == "" {
				log.Error(nil, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC error: tracedata:", data, "format error")
				return nil, cerror.ErrReq
			} else if clientdeep, e := strconv.Atoi(data[4]); e != nil {
				log.Error(nil, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC error: tracedata:", data, "format error")
				return nil, cerror.ErrReq
			} else {
				ctx = trace.InitTrace(ctx, data[0], s.selfappname, host.Hostip, "GRPC", path, clientdeep)
				sourceapp = data[1]
				sourcemethod = data[2]
				sourcepath = data[3]
			}
		}
		traceid, _, _, _, _, selfdeep = trace.GetTrace(ctx)
		var mdata map[string]string
		if ok {
			data := grpcmetadata.Get("core_metadata")
			if len(data) != 0 {
				mdata = make(map[string]string)
				if e := json.Unmarshal(common.Str2byte(data[0]), &mdata); e != nil {
					log.Error(nil, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC metadata:", data[0], "format error:", e)
					return nil, cerror.ErrReq
				}
			}
		}
		start := time.Now()
		servertimeout := s.getHandlerTimeout(path)
		if servertimeout != 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, start.Add(servertimeout))
			defer cancel()
		}
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
				log.Error(workctx, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				workctx.e = cerror.ErrPanic
				workctx.resp = nil
			}
			end := time.Now()
			trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), trace.SERVER, s.selfappname, host.Hostip, "GRPC", path, &start, &end, workctx.e)
			resp = workctx.resp
			if workctx.e != nil {
				e = workctx.e
			}
		}()
		workctx.run()
		return
	}
}
func (s *CGrpcServer) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}
func (s *CGrpcServer) HandleRPC(context.Context, stats.RPCStats) {
}

type serverconnkey struct{}

func (s *CGrpcServer) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, serverconnkey{}, info)
}
func (s *CGrpcServer) HandleConn(ctx context.Context, stat stats.ConnStats) {
	info, ok := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
	if !ok {
		return
	}
	switch stat.(type) {
	case *stats.ConnBegin:
		atomic.AddInt32(&s.clientnum, 1)
		log.Info(nil, "[cgrpc.server] client:", info.RemoteAddr.String(), "online")
	case *stats.ConnEnd:
		atomic.AddInt32(&s.clientnum, -1)
		log.Info(nil, "[cgrpc.server] client:", info.RemoteAddr.String(), "offline")
	}
}
