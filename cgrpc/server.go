package cgrpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	cmetadata "github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/experimental"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

type OutsideHandler func(*ServerContext)

type ServerConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the GlobalTimeout
	//the client's deadline will also effect the rpc call's final deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

func (c *ServerConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(3 * time.Second)
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
	}
	if c.HeartProbe <= 0 {
		c.HeartProbe = ctime.Duration(time.Second * 5)
	} else if c.HeartProbe.StdDuration() < time.Second {
		c.HeartProbe = ctime.Duration(time.Second)
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
}

type CGrpcServer struct {
	c              *ServerConfig
	self           string
	global         []OutsideHandler
	server         *grpc.Server
	clientnum      int32
	reqnum         int64
	services       map[string]*grpc.ServiceDesc
	handlerTimeout map[string]time.Duration
}

// if tlsc is not nil,the tls will be actived
func NewCGrpcServer(c *ServerConfig, selfproject, selfgroup, selfapp string, tlsc *tls.Config) (*CGrpcServer, error) {
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[cgrpc.NewCGrpcServer] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	//pre check
	selffullname, e := name.MakeFullName(selfproject, selfgroup, selfapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:              c,
		self:           selffullname,
		global:         make([]OutsideHandler, 0),
		services:       make(map[string]*grpc.ServiceDesc),
		handlerTimeout: make(map[string]time.Duration),
	}
	opts := make([]grpc.ServerOption, 0, 10)
	opts = append(opts, experimental.BufferPool(bpool.GetPool()))
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.StatsHandler(serverinstance))
	opts = append(opts, grpc.UnknownServiceHandler(func(_ interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		rpcinfo := ctx.Value(serverrpckey{}).(*stats.RPCTagInfo)
		gmd, ok := gmetadata.FromIncomingContext(ctx)
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
		slog.ErrorContext(nil, "[cgrpc.server] path doesn't exist", slog.String("cip", peerip), slog.String("path", rpcinfo.FullMethodName))
		return cerror.ErrNoapi
	}))
	opts = append(opts, grpc.ConnectionTimeout(c.ConnectTimeout.StdDuration()))
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle: c.IdleTimeout.StdDuration(),
		Time:              c.HeartProbe.StdDuration(),
		Timeout:           c.HeartProbe.StdDuration() * 3,
	}))
	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: time.Millisecond * 999, PermitWithoutStream: true}))
	if tlsc != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsc)))
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
	return this.reqnum
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (s *CGrpcServer) StopCGrpcServer(force bool) {
	if force {
		s.server.Stop()
	} else {
		s.server.GracefulStop()
	}
}

// first key path,second key method,value timeout(if timeout <= 0 means no timeout)
func (this *CGrpcServer) UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmp := make(map[string]time.Duration)
	for path := range timeout {
		for method, to := range timeout[path] {
			if method != "GRPC" {
				continue
			}
			if path == "" {
				continue
			}
			if path[0] != '/' {
				path = "/" + path
			}
			tmp[path] = to.StdDuration()
		}
	}
	this.handlerTimeout = tmp
}

func (this *CGrpcServer) getHandlerTimeout(path string) time.Duration {
	if t, ok := this.handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout.StdDuration()
}

// thread unsafe
func (s *CGrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *CGrpcServer) RegisterHandler(sname, mname string, clientstream, serverstream bool, handlers ...OutsideHandler) {
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
	if clientstream || serverstream {
		service.Streams = append(service.Streams, grpc.StreamDesc{
			StreamName:    mname,
			Handler:       s.streamhandler(sname, mname, handlers...),
			ClientStreams: clientstream,
			ServerStreams: serverstream,
		})
	} else {
		service.Methods = append(service.Methods, grpc.MethodDesc{
			MethodName: mname,
			Handler:    s.pingponghandler(sname, mname, handlers...),
		})
	}
}
func (s *CGrpcServer) pingponghandler(sname, mname string, handlers ...OutsideHandler) func(interface{}, context.Context, func(any) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ interface{}, basectx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (resp interface{}, e error) {
		gmd, ok := gmetadata.FromIncomingContext(basectx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != s.self {
				return nil, cerror.ErrTarget
			}
		}
		atomic.AddInt64(&s.reqnum, 1)
		defer atomic.AddInt64(&s.reqnum, -1)
		//trace
		peerip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				peerip = strings.TrimSpace(realip[0])
			}
		}
		if peerip == "" {
			conninfo := basectx.Value(serverconnkey{}).(*stats.ConnTagInfo)
			remoteaddr := conninfo.RemoteAddr.String()
			peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}

		//deal trace data
		var span *trace.Span
		if ok {
			if traceparentstr := gmd.Get("traceparent"); len(traceparentstr) != 0 && traceparentstr[0] != "" {
				tid, psid, e := trace.ParseTraceParent(traceparentstr[0])
				if e != nil {
					slog.ErrorContext(nil, "[cgrpc.server] trace data format wrong",
						slog.String("cip", peerip),
						slog.String("path", path),
						slog.String("trace_parent", traceparentstr[0]))
					return nil, cerror.ErrReq
				}
				parent := trace.NewSpanData(tid, psid)
				if tracestatestr := gmd.Get("tracestate"); len(tracestatestr) != 0 && tracestatestr[0] != "" {
					tmp, e := trace.ParseTraceState(tracestatestr[0])
					if e != nil {
						slog.ErrorContext(nil, "[cgrpc.server] trace data format wrong",
							slog.String("cip", peerip),
							slog.String("path", path),
							slog.String("trace_state", tracestatestr[0]))
						return nil, cerror.ErrReq
					}
					var app, host, method, path bool
					for k, v := range tmp {
						switch k {
						case "app":
							app = true
						case "host":
							host = true
							peerip = v
						case "method":
							method = true
						case "path":
							path = true
						}
						parent.SetStateKV(k, v)
					}
					if !app {
						parent.SetStateKV("app", "unknown")
					}
					if !host {
						parent.SetStateKV("host", peerip)
					}
					if !method {
						parent.SetStateKV("method", "unknown")
					}
					if !path {
						parent.SetStateKV("path", "unknown")
					}
				}
				basectx, span = trace.NewSpan(basectx, "Corelib.CGrpc", trace.Server, parent)
			} else {
				basectx, span = trace.NewSpan(basectx, "Corelib.CGrpc", trace.Server, nil)
				span.GetParentSpanData().SetStateKV("app", "unknown")
				span.GetParentSpanData().SetStateKV("host", peerip)
				span.GetParentSpanData().SetStateKV("method", "unknown")
				span.GetParentSpanData().SetStateKV("path", "unknown")
			}
			span.GetSelfSpanData().SetStateKV("app", s.self)
			span.GetSelfSpanData().SetStateKV("host", host.Hostip)
			span.GetSelfSpanData().SetStateKV("method", "GRPC")
			span.GetSelfSpanData().SetStateKV("path", path)
		}

		//deal metadata
		var md map[string]string
		if ok {
			data := gmd.Get("Core-Metadata")
			if len(data) != 0 {
				md = make(map[string]string)
				if e := json.Unmarshal(common.STB(data[0]), &md); e != nil {
					slog.ErrorContext(basectx, "[cgrpc.server] metadata format wrong",
						slog.String("cip", peerip),
						slog.String("path", path),
						slog.String("metadata", data[0]))
					return nil, cerror.ErrReq
				}
			}
		}
		if md == nil {
			md = map[string]string{"Client-IP": peerip}
		} else if _, ok := md["Client-IP"]; !ok {
			md["Client-IP"] = peerip
		}
		basectx = cmetadata.SetMetadata(basectx, md)

		//timeout
		servertimeout := s.getHandlerTimeout(path)
		if servertimeout > 0 {
			var cancel context.CancelFunc
			basectx, cancel = context.WithDeadline(basectx, time.Now().Add(servertimeout))
			defer cancel()
		}
		workctx := &ServerContext{
			Context:    basectx,
			decodefunc: decode,
			path:       path,
			peerip:     peerip,
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				slog.ErrorContext(workctx, "[cgrpc.server] panic",
					slog.String("cip", peerip),
					slog.String("path", path),
					slog.Any("panic", e),
					slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				workctx.Abort(cerror.ErrPanic)
			}
			grpc.SetTrailer(basectx, gmetadata.New(map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}))
			span.Finish(workctx.e)
			peername, _ := span.GetParentSpanData().GetStateKV("app")
			monitor.GrpcServerMonitor(peername, "GRPC", path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
			if workctx.e != nil {
				resp = nil
				e = workctx.e
			} else {
				resp = workctx.resp
				e = nil
			}
		}()
		for _, handler := range totalhandlers {
			handler(workctx)
			if workctx.finish == 1 {
				break
			}
		}
		return
	}
}
func (s *CGrpcServer) streamhandler(sname, mname string, handlers ...OutsideHandler) func(srv any, stream grpc.ServerStream) error {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ any, stream grpc.ServerStream) (e error) {
		basectx := stream.Context()
		gmd, ok := gmetadata.FromIncomingContext(basectx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != s.self {
				return cerror.ErrTarget
			}
		}
		atomic.AddInt64(&s.reqnum, 1)
		defer atomic.AddInt64(&s.reqnum, -1)
		//trace
		peerip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				peerip = strings.TrimSpace(realip[0])
			}
		}
		if peerip == "" {
			conninfo := basectx.Value(serverconnkey{}).(*stats.ConnTagInfo)
			remoteaddr := conninfo.RemoteAddr.String()
			peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}

		//deal trace data
		var span *trace.Span
		if ok {
			if traceparentstr := gmd.Get("traceparent"); len(traceparentstr) != 0 && traceparentstr[0] != "" {
				tid, psid, e := trace.ParseTraceParent(traceparentstr[0])
				if e != nil {
					slog.ErrorContext(nil, "[cgrpc.server] trace data format wrong",
						slog.String("cip", peerip),
						slog.String("path", path),
						slog.String("trace_parent", traceparentstr[0]))
					return cerror.ErrReq
				}
				parent := trace.NewSpanData(tid, psid)
				if tracestatestr := gmd.Get("tracestate"); len(tracestatestr) != 0 && tracestatestr[0] != "" {
					tmp, e := trace.ParseTraceState(tracestatestr[0])
					if e != nil {
						slog.ErrorContext(nil, "[cgrpc.server] trace data format wrong",
							slog.String("cip", peerip),
							slog.String("path", path),
							slog.String("trace_state", tracestatestr[0]))
						return cerror.ErrReq
					}
					var app, host, method, path bool
					for k, v := range tmp {
						switch k {
						case "app":
							app = true
						case "host":
							host = true
							peerip = v
						case "method":
							method = true
						case "path":
							path = true
						}
						parent.SetStateKV(k, v)
					}
					if !app {
						parent.SetStateKV("app", "unknown")
					}
					if !host {
						parent.SetStateKV("host", peerip)
					}
					if !method {
						parent.SetStateKV("method", "unknown")
					}
					if !path {
						parent.SetStateKV("path", "unknown")
					}
				}
				basectx, span = trace.NewSpan(basectx, "Corelib.CGrpc", trace.Server, parent)
			} else {
				basectx, span = trace.NewSpan(basectx, "Corelib.CGrpc", trace.Server, nil)
				span.GetParentSpanData().SetStateKV("app", "unknown")
				span.GetParentSpanData().SetStateKV("host", peerip)
				span.GetParentSpanData().SetStateKV("method", "unknown")
				span.GetParentSpanData().SetStateKV("path", "unknown")
			}
			span.GetSelfSpanData().SetStateKV("app", s.self)
			span.GetSelfSpanData().SetStateKV("host", host.Hostip)
			span.GetSelfSpanData().SetStateKV("method", "GRPC")
			span.GetSelfSpanData().SetStateKV("path", path)
		}

		//deal metadata
		var md map[string]string
		if ok {
			data := gmd.Get("Core-Metadata")
			if len(data) != 0 {
				md = make(map[string]string)
				if e := json.Unmarshal(common.STB(data[0]), &md); e != nil {
					slog.ErrorContext(basectx, "[cgrpc.server] metadata format wrong",
						slog.String("cip", peerip),
						slog.String("path", path),
						slog.String("metadata", data[0]))
					return cerror.ErrReq
				}
			}
		}
		if md == nil {
			md = map[string]string{"Client-IP": peerip}
		} else if _, ok := md["Client-IP"]; !ok {
			md["Client-IP"] = peerip
		}
		basectx = cmetadata.SetMetadata(basectx, md)

		//timeout
		servertimeout := s.getHandlerTimeout(path)
		if servertimeout > 0 {
			var cancel context.CancelFunc
			basectx, cancel = context.WithDeadline(basectx, time.Now().Add(servertimeout))
			defer cancel()
		}
		workctx := &ServerContext{
			Context: basectx,
			stream:  stream,
			path:    path,
			peerip:  peerip,
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				slog.ErrorContext(workctx, "[cgrpc.server] panic",
					slog.String("cip", peerip),
					slog.String("path", path),
					slog.Any("panic", e),
					slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				workctx.Abort(cerror.ErrPanic)
			}
			grpc.SetTrailer(basectx, gmetadata.New(map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}))
			span.Finish(workctx.e)
			peername, _ := span.GetParentSpanData().GetStateKV("app")
			monitor.GrpcServerMonitor(peername, "GRPC", path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
			e = workctx.e
		}()
		for _, handler := range totalhandlers {
			handler(workctx)
			if workctx.finish == 1 {
				break
			}
		}
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
	gmd, ok := gmetadata.FromIncomingContext(ctx)
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
		slog.InfoContext(nil, "[cgrpc.server] online", slog.String("cip", peerip))
	case *stats.ConnEnd:
		atomic.AddInt32(&s.clientnum, -1)
		slog.InfoContext(nil, "[cgrpc.server] offline", slog.String("cip", peerip))
	}
}
