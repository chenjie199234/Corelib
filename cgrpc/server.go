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
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/internal/version"
	cmetadata "github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
)

type OutsideHandler func(*ServerContext)

type ServerConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the GlobalTimeout
	//the client's deadline will also effect the rpc call's final deadline
	DefaultHandlerTimeout ctime.Duration `json:"default_handler_timeout"`
	//time for connection establish(include dial time,tls handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//time for waiting the next message,it will be reset when starting to read next message
	//expired without next message,the connection will be closed
	//<=0 means no timeout
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
	handlerTimeout atomic.Pointer[map[string]time.Duration]

	statusCounter metric.Int64Counter
	timeHistogram metric.Float64Histogram
	tracer        trace.Tracer
}

// if tlsc is not nil,the tls will be activated
func NewCGrpcServer(c *ServerConfig, tlsc *tls.Config) (*CGrpcServer, error) {
	if e := cotel.Init(); e != nil {
		return nil, e
	}
	sCounter, e := otel.Meter("Corelib.cgrpc.server", metric.WithInstrumentationVersion(version.String())).Int64Counter("cgrpc.server.path.status", metric.WithUnit("1"))
	if e != nil {
		return nil, e
	}
	tHistogram, e := otel.Meter("Corelib.cgrpc.server", metric.WithInstrumentationVersion(version.String())).Float64Histogram("cgrpc.server.path.time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	if e != nil {
		return nil, e
	}
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[cgrpc.server] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:             c,
		global:        make([]OutsideHandler, 0),
		statshandler:  &sStatsHandler{},
		services:      make(map[string]*grpc.ServiceDesc),
		statusCounter: sCounter,
		timeHistogram: tHistogram,
		tracer:        otel.Tracer("Corelib.cgrpc.server", trace.WithInstrumentationVersion(version.String())),
	}
	opts := make([]grpc.ServerOption, 0, 10)
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.StatsHandler(serverinstance.statshandler))
	opts = append(opts, grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		ctx := stream.Context()
		rpcinfo := ctx.Value(serverrpckey{}).(*stats.RPCTagInfo)
		peerip := peerip(ctx)
		slog.Error("[cgrpc.server] path doesn't exist", slog.String("cip", peerip), slog.String("path", rpcinfo.FullMethodName))
		return cerror.ErrNoapi
	}))
	opts = append(opts, grpc.ConnectionTimeout(c.ConnectTimeout.StdDuration()))
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle: c.IdleTimeout.StdDuration(),
		Time:              c.HeartProbe.StdDuration(),
		Timeout:           c.HeartProbe.StdDuration()*3 + c.HeartProbe.StdDuration()/3, //give 1/3 HeartProbe for netlag
	}))
	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: time.Minute, PermitWithoutStream: true}))
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

// first key path,second key method(must be GRPC),value timeout(if timeout <= 0 means no timeout)
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
	this.handlerTimeout.Store(&tmp)
}

func (this *CGrpcServer) getHandlerTimeout(path string) time.Duration {
	tmp := this.handlerTimeout.Load()
	if tmp == nil {
		return this.c.DefaultHandlerTimeout.StdDuration()
	}
	if t, ok := (*tmp)[path]; ok {
		return t
	}
	return this.c.DefaultHandlerTimeout.StdDuration()
}

// thread unsafe
func (s *CGrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe,this must be called before StartCGrpcServer
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
			Handler:    s.echohandler(sname, mname, handlers...),
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

func (s *CGrpcServer) createwctx(basectx context.Context, path string, sto time.Duration) (*ServerContext, trace.Span, bool, error) {
	gmd, ok := gmetadata.FromIncomingContext(basectx)
	if ok {
		if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != name.GetSelfFullName() {
			return nil, nil, false, cerror.ErrTarget
		}
	}

	peerip := peerip(basectx)

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
	var span trace.Span
	if ok {
		basectx, span = s.tracer.Start(
			otel.GetTextMapPropagator().Extract(basectx, wrapmetadata(gmd)),
			"handle grpc",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("path", path), attribute.String("cname", clientname), attribute.String("cip", peerip)))
	} else {
		basectx, span = s.tracer.Start(basectx, "handle grpc", trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("path", path), attribute.String("cname", clientname), attribute.String("cip", peerip)))
	}

	var stime int64 //only used in metric
	if cotel.NeedMetric() {
		if cotel.NeedTrace() {
			stime = span.(sdktrace.ReadOnlySpan).StartTime().UnixNano()
		} else {
			stime = time.Now().UnixNano()
		}
	}

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
				return nil, nil, false, cerror.ErrReq
			}
		}
	}
	if md == nil {
		md = map[string]string{"Client-IP": peerip}
	} else if _, ok := md["Client-IP"]; !ok {
		md["Client-IP"] = peerip
	}
	basectx = cmetadata.SetMetadata(basectx, md)

	//only when the server's deadline < client's deadline
	earlyreturn := false

	//timeout
	var basecancel context.CancelFunc
	if sto > 0 {
		cdl, cok := basectx.Deadline()
		basectx, basecancel = context.WithDeadline(basectx, time.Now().Add(sto))
		dl, _ := basectx.Deadline()
		earlyreturn = !cok || dl.Before(cdl)
	}
	return &ServerContext{
		Context: basectx,
		cancel:  basecancel,
		stime:   stime,
		path:    path,
		peerip:  peerip,
	}, span, earlyreturn, nil
}
func (s *CGrpcServer) handler(wctx *ServerContext, span trace.Span, totalhandlers []OutsideHandler) {
	defer func() {
		if e := recover(); e != nil {
			stack := make([]byte, 1024)
			n := runtime.Stack(stack, false)
			slog.ErrorContext(wctx, "[cgrpc.server] panic",
				slog.String("cip", wctx.peerip),
				slog.String("path", wctx.path),
				slog.Any("panic", e),
				slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
			wctx.Abort(cerror.ErrPanic)
		}
		if wctx.e != nil {
			span.SetStatus(codes.Error, wctx.e.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		if cotel.NeedMetric() {
			var etime int64
			if cotel.NeedTrace() {
				etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
			} else {
				etime = time.Now().UnixNano()
			}
			attr := attribute.String("path", wctx.path)
			if wctx.e != nil {
				s.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "error")))
			} else {
				s.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "ok")))
			}
			s.timeHistogram.Record(context.Background(), float64(etime-wctx.stime)/1000000.0, metric.WithAttributes(attr))
		}
	}()
	for _, handler := range totalhandlers {
		handler(wctx)
		if wctx.closed.Load() {
			break
		}
	}
	if !wctx.closed.Load() {
		wctx.Abort(nil)
	}
}
func (s *CGrpcServer) deadlinehandler(wctx *ServerContext, span trace.Span, totalhandlers []OutsideHandler) (earlyreturn bool) {
	done := make(chan struct{})
	go func() {
		s.handler(wctx, span, totalhandlers)
		close(done)
	}()
	select {
	case <-wctx.Context.Done():
		switch wctx.Context.Err() {
		case context.DeadlineExceeded:
			//need to early return
			wctx.closed.Swap(true)
			return true
		case context.Canceled:
			//only when the client leave,the connection close will enter this
			//the client already gone,we don't need to return early,wait the handler return here
			<-done
		}
	case <-done:
	}
	return false
}
func (s *CGrpcServer) echohandler(sname, mname string, handlers ...OutsideHandler) func(any, context.Context, func(any) error, grpc.UnaryServerInterceptor) (any, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		wctx, span, earlyreturn, e := s.createwctx(ctx, path, s.getHandlerTimeout(path))
		if e != nil {
			return nil, e
		}
		if wctx.cancel != nil {
			defer wctx.cancel()
		}
		wctx.decodefunc = decode
		if !earlyreturn {
			//run in sync mode
			s.handler(wctx, span, totalhandlers)
		} else {
			//run in async mode
			if s.deadlinehandler(wctx, span, totalhandlers) {
				return nil, cerror.ErrDeadlineExceeded
			}
		}
		//fix the interface nil problem
		if wctx.e != nil {
			return nil, wctx.e
		}
		return wctx.resp, nil
	}
}
func (s *CGrpcServer) streamhandler(sname, mname string, handlers ...OutsideHandler) func(srv any, stream grpc.ServerStream) error {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ any, stream grpc.ServerStream) (err error) {
		wctx, span, earlyreturn, e := s.createwctx(stream.Context(), path, s.getHandlerTimeout(path))
		if e != nil {
			return e
		}
		if wctx.cancel != nil {
			defer wctx.cancel()
		}
		wctx.stream = stream
		if !earlyreturn {
			//run in sync mode
			s.handler(wctx, span, totalhandlers)
		} else {
			//run in async mode
			if s.deadlinehandler(wctx, span, totalhandlers) {
				return cerror.ErrDeadlineExceeded
			}
		}
		return wctx.e
	}
}
func peerip(ctx context.Context) string {
	gmd, ok := gmetadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	peerip := ""
	if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
		peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
	} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
		peerip = strings.TrimSpace(realip[0])
	}
	if peerip == "" {
		p, ok := peer.FromContext(ctx)
		if !ok {
			return ""
		}
		host, _, e := net.SplitHostPort(p.Addr.String())
		if e != nil {
			peerip = p.Addr.String()
		} else {
			peerip = host
		}
	}
	return peerip
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

func (s *sStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (s *sStatsHandler) HandleConn(ctx context.Context, stat stats.ConnStats) {
	peerip := peerip(ctx)
	switch stat.(type) {
	case *stats.ConnBegin:
		s.clientnum.Add(1)
		slog.Info("[cgrpc.server] online", slog.String("cip", peerip))
	case *stats.ConnEnd:
		s.clientnum.Add(-1)
		slog.Info("[cgrpc.server] offline", slog.String("cip", peerip))
	}
}
