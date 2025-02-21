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
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	global         []OutsideHandler
	server         *grpc.Server
	statshandler   *sStatsHandler
	services       map[string]*grpc.ServiceDesc
	handlerTimeout map[string]time.Duration
}

// if tlsc is not nil,the tls will be actived
func NewCGrpcServer(c *ServerConfig, tlsc *tls.Config) (*CGrpcServer, error) {
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[cgrpc.server] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	if e := name.HasSelfFullName(); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:              c,
		global:         make([]OutsideHandler, 0),
		statshandler:   &sStatsHandler{},
		services:       make(map[string]*grpc.ServiceDesc),
		handlerTimeout: make(map[string]time.Duration),
	}
	opts := make([]grpc.ServerOption, 0, 10)
	opts = append(opts, experimental.BufferPool(bpool.GetGrpcPool()))
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.StatsHandler(serverinstance.statshandler))
	opts = append(opts, grpc.UnknownServiceHandler(func(_ interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		rpcinfo := ctx.Value(serverrpckey{}).(*stats.RPCTagInfo)
		peerip := ctx.Value(serverconnkey{}).(string)
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
	return this.statshandler.clientnum.Load()
}
func (this *CGrpcServer) GetReqNum() int64 {
	return this.statshandler.reqnum.Load()
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

type wrapmetadata gmetadata.MD

func (md wrapmetadata) Get(k string) string {
	data := (gmetadata.MD)(md).Get(k)
	if len(data) == 0 {
		return ""
	}
	return data[0]
}
func (md wrapmetadata) Set(k, v string) {
	(gmetadata.MD)(md).Set(k, v)
}
func (md wrapmetadata) Keys() []string {
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	return keys
}
func (s *CGrpcServer) pingponghandler(sname, mname string, handlers ...OutsideHandler) func(interface{}, context.Context, func(any) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ interface{}, basectx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (resp interface{}, err error) {
		gmd, ok := gmetadata.FromIncomingContext(basectx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != name.GetSelfFullName() {
				return nil, cerror.ErrTarget
			}
		}

		peerip := basectx.Value(serverconnkey{}).(string)

		//trace
		clientname := "unknown"
		tracedata := make(map[string]string)
		if ok {
			if data := gmd.Get("Core-Self"); len(data) != 0 && len(data[0]) != 0 {
				clientname = data[0]
			}
			if data := gmd.Get("Traceparent"); len(data) != 0 && len(data[0]) != 0 {
				tracedata["Traceparent"] = data[0]
			}
			if data := gmd.Get("Tracestate"); len(data) != 0 && len(data[0]) != 0 {
				tracedata["Tracestate"] = data[0]
			}
		}
		basectx, span := otel.Tracer(name.GetSelfFullName()).Start(
			otel.GetTextMapPropagator().Extract(basectx, wrapmetadata(gmd)),
			"handle grpc",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("url.path", path), attribute.String("client.name", clientname), attribute.String("client.ip", peerip)))

		//metadata
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
			//fix the interface nil problem
			if workctx.e != nil {
				span.SetStatus(codes.Error, workctx.e.Error())
				resp = nil
				err = workctx.e
			} else {
				span.SetStatus(codes.Ok, "")
				resp = workctx.resp
				err = nil
			}
			span.End()
			// monitor.GrpcServerMonitor(peername, "GRPC", path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
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
	return func(_ any, stream grpc.ServerStream) (err error) {
		basectx := stream.Context()
		gmd, ok := gmetadata.FromIncomingContext(basectx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != name.GetSelfFullName() {
				return cerror.ErrTarget
			}
		}

		peerip := basectx.Value(serverconnkey{}).(string)

		//trace
		clientname := "unknown"
		if ok {
			if data := gmd.Get("Core-Self"); len(data) != 0 && len(data[0]) != 0 {
				clientname = data[0]
			}
		}
		basectx, span := otel.Tracer(name.GetSelfFullName()).Start(
			otel.GetTextMapPropagator().Extract(basectx, wrapmetadata(gmd)),
			"handle grpc",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("url.path", path), attribute.String("client.name", clientname), attribute.String("client.ip", peerip)))

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
			//fix the interface nil problem
			if workctx.e != nil {
				span.SetStatus(codes.Error, workctx.e.Error())
				err = workctx.e
			} else {
				span.SetStatus(codes.Ok, "")
				err = nil
			}
			span.End()
			// monitor.GrpcServerMonitor(peername, "GRPC", path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
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

type sStatsHandler struct {
	clientnum atomic.Int32
	reqnum    atomic.Int64
}

type serverrpckey struct{}

func (s *sStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	s.reqnum.Add(1)
	return context.WithValue(ctx, serverrpckey{}, info)
}
func (s *sStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	switch rs.(type) {
	case *stats.End:
		s.reqnum.Add(-1)
	}
}

type serverconnkey struct{}

func (s *sStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
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
	return context.WithValue(ctx, serverconnkey{}, peerip)
}
func (s *sStatsHandler) HandleConn(ctx context.Context, stat stats.ConnStats) {
	peerip := ctx.Value(serverconnkey{}).(string)
	switch stat.(type) {
	case *stats.ConnBegin:
		s.clientnum.Add(1)
		slog.InfoContext(nil, "[cgrpc.server] online", slog.String("cip", peerip))
	case *stats.ConnEnd:
		s.clientnum.Add(-1)
		slog.InfoContext(nil, "[cgrpc.server] offline", slog.String("cip", peerip))
	}
}
