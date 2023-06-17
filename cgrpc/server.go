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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	GlobalTimeout  time.Duration     //global timeout for every rpc call,<=0 means no timeout
	ConnectTimeout time.Duration     //default 500ms
	HeartPorbe     time.Duration     //default 10s,min 10s
	MaxMsgLen      uint32            //default 64M,min 64k
	Certs          map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
	}
	if c.HeartPorbe < time.Second*10 {
		c.HeartPorbe = time.Second * 10
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
}

type CGrpcServer struct {
	c              *ServerConfig
	selfapp        string //group.name
	global         []OutsideHandler
	ctxpool        *sync.Pool
	server         *grpc.Server
	clientnum      int32
	services       map[string]*grpc.ServiceDesc
	handlerTimeout map[string]time.Duration
	totalreqnum    int64
}

func NewCGrpcServer(c *ServerConfig, selfappgroup, selfappname string) (*CGrpcServer, error) {
	selfapp := selfappgroup + "." + selfappname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:              c,
		selfapp:        selfapp,
		global:         make([]OutsideHandler, 0),
		ctxpool:        &sync.Pool{},
		services:       make(map[string]*grpc.ServiceDesc),
		handlerTimeout: make(map[string]time.Duration),
	}
	opts := make([]grpc.ServerOption, 0, 6)
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.StatsHandler(serverinstance))
	opts = append(opts, grpc.UnknownServiceHandler(func(_ interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		rpcinfo := ctx.Value(serverrpckey{}).(*stats.RPCTagInfo)
		gmd, ok := metadata.FromIncomingContext(ctx)
		peerip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				peerip = strings.TrimSpace(realip[0])
			}
		}
		if peerip == "" {
			conninfo := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
			remoteaddr := conninfo.RemoteAddr.String()
			peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}
		log.Error(nil, "[cgrpc.server] client:", peerip, "call path:", rpcinfo.FullMethodName, "doesn't exist")
		return cerror.ErrNoapi
	}))
	if c.ConnectTimeout != 0 {
		opts = append(opts, grpc.ConnectionTimeout(c.ConnectTimeout))
	}
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{Time: c.HeartPorbe, Timeout: c.HeartPorbe*3 + c.HeartPorbe/3}))
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[cgrpc.server] load cert: " + cert + " key: " + key + " " + e.Error())
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
		return errors.New("[cgrpc.server] listen tcp addr: " + listenaddr + " " + e.Error())
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
func (this *CGrpcServer) GetClientNum() int32 {
	return this.clientnum
}
func (this *CGrpcServer) GetReqNum() int64 {
	return this.totalreqnum
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (s *CGrpcServer) StopCGrpcServer(force bool) {
	if force {
		s.server.Stop()
	} else {
		s.server.GracefulStop()
	}
}

// key path,value timeout(if timeout <= 0 means no timeout)
func (this *CGrpcServer) UpdateHandlerTimeout(htcs map[string]time.Duration) {
	tmp := make(map[string]time.Duration)
	for path, timeout := range htcs {
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}
		tmp[path] = timeout
	}
	this.handlerTimeout = tmp
}

func (this *CGrpcServer) getHandlerTimeout(path string) time.Duration {
	if t, ok := this.handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout
}

// thread unsafe
func (s *CGrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
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
		gmd, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != s.selfapp {
				return nil, cerror.ErrTarget
			}
		}
		atomic.AddInt64(&s.totalreqnum, 1)
		defer atomic.AddInt64(&s.totalreqnum, -1)
		conninfo := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
		remoteaddr := conninfo.RemoteAddr.String()
		localaddr := conninfo.LocalAddr.String()
		sourceip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				sourceip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				sourceip = strings.TrimSpace(realip[0])
			}
		}
		if sourceip == "" {
			sourceip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		if ok {
			if data := gmd.Get("Core-Tracedata"); len(data) == 0 || data[0] == "" {
				ctx = log.InitTrace(ctx, "", s.selfapp, host.Hostip, "GRPC", path, 0)
			} else if len(data) != 5 {
				log.Error(nil, "[cgrpc.server] client:", sourceip, "path:", path, "method: GRPC error: tracedata:", data, "format wrong")
				return nil, cerror.ErrReq
			} else {
				sourceapp = data[1]
				sourcemethod = data[2]
				sourcepath = data[3]
				clientdeep, e := strconv.Atoi(data[4])
				if e != nil || sourceapp == "" || sourcemethod == "" || sourcepath == "" || clientdeep == 0 {
					log.Error(nil, "[cgrpc.server] client:", sourceip, "path:", path, "method: GRPC error: tracedata:", data, "format wrong")
					return nil, cerror.ErrReq
				}
				ctx = log.InitTrace(ctx, data[0], s.selfapp, host.Hostip, "GRPC", path, clientdeep)
			}
		}
		traceid, _, _, _, _, selfdeep := log.GetTrace(ctx)
		var mdata map[string]string
		if ok {
			data := gmd.Get("Core-Metadata")
			if len(data) != 0 {
				mdata = make(map[string]string)
				if e := json.Unmarshal(common.Str2byte(data[0]), &mdata); e != nil {
					log.Error(ctx, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC metadata:", data[0], "format wrong:", e)
					return nil, cerror.ErrReq
				}
			}
		}
		start := time.Now()
		servertimeout := s.getHandlerTimeout(path)
		if servertimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, start.Add(servertimeout))
			defer cancel()
		}
		workctx := s.getcontext(ctx, path, sourceapp, remoteaddr, mdata, totalhandlers, decode)
		if _, ok := workctx.metadata["Client-IP"]; !ok {
			workctx.metadata["Client-IP"] = sourceip
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[cgrpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: GRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				workctx.e = cerror.ErrPanic
				workctx.resp = nil
			}
			end := time.Now()
			log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfapp, host.Hostip+":"+localaddr[strings.LastIndex(localaddr, ":")+1:], "GRPC", path, &start, &end, workctx.e)
			monitor.GrpcServerMonitor(sourceapp, "GRPC", path, workctx.e, uint64(end.UnixNano()-start.UnixNano()))
			resp = workctx.resp
			if workctx.e != nil {
				e = workctx.e
			}
		}()
		workctx.run()
		return
	}
}

type serverrpckey struct{}

func (s *CGrpcServer) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, serverrpckey{}, info)
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
	gmd, ok := metadata.FromIncomingContext(ctx)
	peerip := ""
	if ok {
		if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
			peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
		} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
			peerip = strings.TrimSpace(realip[0])
		}
	}
	if peerip == "" {
		remoteaddr := info.RemoteAddr.String()
		peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
	}
	switch stat.(type) {
	case *stats.ConnBegin:
		atomic.AddInt32(&s.clientnum, 1)
		log.Info(nil, "[cgrpc.server] client:", peerip, "online")
	case *stats.ConnEnd:
		atomic.AddInt32(&s.clientnum, -1)
		log.Info(nil, "[cgrpc.server] client:", peerip, "offline")
	}
}
