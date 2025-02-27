package crpc

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

type ClientConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if ctx's Deadline exist and GlobalTimeout > 0,the min(time.Now().Add(GlobalTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and GlobalTimeout > 0 ,the time.Now().Add(GlobalTimeout) will be used as the final deadline
	//if ctx's deadline not exist and GlobalTimeout <=0,means no deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establich(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout,if >0 min is HeartProbe
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

type CrpcClient struct {
	serverfullname string
	c              *ClientConfig
	tlsc           *tls.Config
	instance       *stream.Instance

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop    *graceful.Graceful
	reqpool *sync.Pool
}

// if tlsc is not nil,the tls will be actived
func NewCrpcClient(c *ClientConfig, d discover.DI, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*CrpcClient, error) {
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[crpc.client] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	if e := name.HasSelfFullName(); e != nil {
		return nil, e
	}
	serverfullname, e := name.MakeFullName(serverproject, servergroup, serverapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ClientConfig{}
	}
	if d == nil {
		return nil, errors.New("[crpc.client] missing discover")
	}
	if !d.CheckTarget(serverfullname) {
		return nil, errors.New("[crpc.client] discover's target app not match")
	}
	client := &CrpcClient{
		serverfullname: serverfullname,
		c:              c,
		tlsc:           tlsc,

		discover: d,

		reqpool: &sync.Pool{},
		stop:    graceful.New(),
	}
	instancec := &stream.InstanceConfig{
		RecvIdleTimeout:    c.IdleTimeout.StdDuration(),
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout.StdDuration(),
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	//tcp instalce
	instancec.VerifyFunc = client.verifyfunc
	instancec.OnlineFunc = client.onlinefunc
	instancec.UserdataFunc = client.userfunc
	instancec.OfflineFunc = client.offlinefunc
	client.instance, _ = stream.NewInstance(instancec)

	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Crpc)
	client.resolver.Start()
	return client, nil
}

func (c *CrpcClient) ResolveNow() {
	go c.resolver.Now()
}

// get the server's addrs from the discover.DI(the param in NewCrpcClient)
// version can be int64 or string(should only be used with == or !=)
func (c *CrpcClient) GetServerIps() (ips []string, version any, lasterror error) {
	tmp, version, e := c.discover.GetAddrs(discover.NotNeed)
	ips = make([]string, 0, len(tmp))
	for k := range tmp {
		ips = append(ips, k)
	}
	lasterror = e
	return
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (c *CrpcClient) Close(force bool) {
	if force {
		c.resolver.Close()
		c.instance.Stop()
	} else {
		c.stop.Close(c.resolver.Close, c.instance.Stop)
	}
}

func (c *CrpcClient) start(server *ServerForPick, reconnect bool) {
	if reconnect && !c.balancer.ReconnectCheck(server) {
		//can't reconnect to server
		return
	}
	if !c.instance.StartClient(server.addr, false, common.STB(c.serverfullname), c.tlsc) {
		go c.start(server, true)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, string, bool) {
	//verify success
	return nil, "", true
}

func (c *CrpcClient) onlinefunc(ctx context.Context, p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.getRegisterServer(p.GetRawConnectAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	server.setpeer(p)
	server.closing = 0
	c.balancer.RebuildPicker(server.addr, true)
	slog.InfoContext(nil, "[crpc.client] online", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		slog.ErrorContext(nil, "[crpc.client] userdata format wrong", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
		return
	}
	switch msg.H.Type {
	case MsgType_CloseRecv:
		if rw := server.getrw(msg.H.Callid); rw != nil {
			rw.status.And(0b0111)
		}
	case MsgType_CloseSend:
		if rw := server.getrw(msg.H.Callid); rw != nil {
			rw.status.And(0b1011)
			rw.reader.Close()
		}
	case MsgType_CloseRecvSend:
		if rw := server.getrw(msg.H.Callid); rw != nil {
			rw.status.And(0b0011)
			rw.reader.Close()
			server.delrw(msg.H.Callid)
		}
		if msg.H.Traildata != nil {
			cpuusage, _ := strconv.ParseFloat(msg.H.Traildata["Cpu-Usage"], 64)
			server.GetServerPickInfo().UpdateCPU(cpuusage)
		}
	case MsgType_Send:
		if msg.B.Error != nil && cerror.Equal(msg.B.Error, cerror.ErrServerClosing) {
			if atomic.SwapInt32(&server.closing, 1) == 0 {
				//set the lowest pick priority
				server.Pickinfo.SetDiscoverServerOffline(0)
				//rebuild picker
				c.balancer.RebuildPicker(server.addr, false)
				//triger discover
				c.resolver.Now()
			}
		}
		if msg.H.Callid != 0 {
			if rw := server.getrw(msg.H.Callid); rw != nil {
				rw.cache(msg.B)
			}
		}
	}
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	slog.InfoContext(nil, "[crpc.client] offline", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
	server.setpeer(nil)
	c.balancer.RebuildPicker(server.addr, false)
	server.cleanrw()
	go c.start(server, true)
}

func (c *CrpcClient) Call(ctx context.Context, path string, in []byte, handler func(ctx *CallContext) error) error {
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return cerror.ErrClientClosing
		}
		return cerror.ErrBusy
	}
	defer c.stop.DoneOne()
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	var deadline int64
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UnixNano()
	}
	md := metadata.GetMetadata(ctx)
	for {
		td := make(map[string]string)
		td["Core-Self"] = name.GetSelfFullName()
		tctx, span := otel.Tracer("Corelib.crpc.client", trace.WithInstrumentationVersion(version.String())).Start(
			ctx,
			"call crpc",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("url.path", path), attribute.String("server.name", c.serverfullname)))
		otel.GetTextMapPropagator().Inject(tctx, propagation.MapCarrier(td))
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			slog.ErrorContext(ctx, "[crpc.client] pick server failed",
				slog.String("sname", c.serverfullname),
				slog.String("path", path),
				slog.String("error", e.Error()))
			span.SetStatus(codes.Error, e.Error())
			span.End()
			c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, true)
			return e
		}
		span.SetAttributes(attribute.String("server.addr", server.addr))
		rw := server.createrw(path, deadline, md, td)
		if e := rw.init(&MsgBody{Body: in}); e != nil {
			server.delrw(rw.callid)
			slog.ErrorContext(ctx, "[crpc.client] send request failed",
				slog.String("sname", c.serverfullname),
				slog.String("sip", server.addr),
				slog.String("path", path),
				slog.String("error", e.Error()))
			server.GetServerPickInfo().Done(false)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cerror.Equal(e, cerror.ErrClosed) {
				continue
			}
			c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, true)
			return e
		}
		workctx := &CallContext{
			Context: ctx,
			rw:      rw,
			s:       server,
		}
		stop := make(chan *struct{})
		go func() {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.Canceled {
					rw.closerecvsend(false, cerror.ErrCanceled)
				} else if ctx.Err() == context.DeadlineExceeded {
					rw.closerecvsend(false, cerror.ErrDeadlineExceeded)
				} else {
					rw.closerecvsend(false, cerror.Convert(ctx.Err()))
				}
			case <-stop:
				rw.closerecvsend(false, nil)
			}
		}()
		ee := cerror.Convert(handler(workctx))
		server.GetServerPickInfo().Done(ee == nil)
		close(stop)
		if ee != nil {
			span.SetStatus(codes.Error, ee.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		server.delrw(rw.callid)
		if cerror.Equal(ee, cerror.ErrServerClosing) {
			continue
		}
		c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, ee != nil)
		//fix the interface not nil problem
		if ee != nil {
			return ee
		}
		return nil
	}
}

func (c *CrpcClient) Stream(ctx context.Context, path string, handler func(ctx *StreamContext) error) error {
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return cerror.ErrClientClosing
		}
		return cerror.ErrBusy
	}
	defer c.stop.DoneOne()
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	var deadline int64
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UnixNano()
	}
	md := metadata.GetMetadata(ctx)
	for {
		td := make(map[string]string)
		td["Core-Self"] = name.GetSelfFullName()
		tctx, span := otel.Tracer("Corelib.crpc.client", trace.WithInstrumentationVersion(version.String())).Start(
			ctx,
			"call crpc",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("url.path", path), attribute.String("server.name", c.serverfullname)))
		otel.GetTextMapPropagator().Inject(tctx, propagation.MapCarrier(td))
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			slog.ErrorContext(ctx, "[crpc.client] pick server failed",
				slog.String("sname", c.serverfullname),
				slog.String("path", path),
				slog.String("error", e.Error()))
			span.SetStatus(codes.Error, e.Error())
			span.End()
			c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, true)
			return e
		}
		span.SetAttributes(attribute.String("server.addr", server.addr))
		rw := server.createrw(path, deadline, md, td)
		if e := rw.init(nil); e != nil {
			server.delrw(rw.callid)
			slog.ErrorContext(ctx, "[crpc.client] init stream failed",
				slog.String("sname", c.serverfullname),
				slog.String("sip", server.addr),
				slog.String("path", path),
				slog.String("error", e.Error()))
			server.GetServerPickInfo().Done(false)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cerror.Equal(e, cerror.ErrClosed) {
				continue
			}
			c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, true)
			return e
		}
		workctx := &StreamContext{
			Context: ctx,
			rw:      rw,
			s:       server,
		}
		stop := make(chan *struct{})
		go func() {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.Canceled {
					rw.closerecvsend(false, cerror.ErrCanceled)
				} else if ctx.Err() == context.DeadlineExceeded {
					rw.closerecvsend(false, cerror.ErrDeadlineExceeded)
				} else {
					rw.closerecvsend(false, cerror.Convert(ctx.Err()))
				}
			case <-stop:
				rw.closerecvsend(false, nil)
			}
		}()
		ee := cerror.Convert(handler(workctx))
		server.GetServerPickInfo().Done(ee == nil)
		close(stop)
		if ee != nil {
			span.SetStatus(codes.Error, ee.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		server.delrw(rw.callid)
		if cerror.Equal(ee, cerror.ErrServerClosing) {
			continue
		}
		c.recordmetric(path, float64(span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()-span.(sdktrace.ReadOnlySpan).StartTime().UnixNano())/1000000.0, ee != nil)
		//fix the interface not nil problem
		if ee != nil {
			return ee
		}
		return nil
	}
}
func (c *CrpcClient) recordmetric(path string, usetimems float64, err bool) {
	mstatus, _ := otel.Meter("Corelib.crpc.client", metric.WithInstrumentationVersion(version.String())).Int64Histogram(path+".status", metric.WithUnit("1"), metric.WithExplicitBucketBoundaries(0))
	if err {
		mstatus.Record(context.Background(), 1)
	} else {
		mstatus.Record(context.Background(), 0)
	}
	mtime, _ := otel.Meter("Corelib.crpc.client", metric.WithInstrumentationVersion(version.String())).Float64Histogram(path+".time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	mtime.Record(context.Background(), usetimems)
}

type forceaddrkey struct{}

// forceaddr: most of the time this should be empty
//
//	if it is not empty,this request will try to transport to this specific addr's server
//	if this specific server doesn't exist,cerror.ErrNoSpecificServer will return
//	if the DI is static:the forceaddr can be addr in the DI's addrs list
//	if the DI is dns:the forceaddr can be addr in the dns resolve result
//	if the DI is kubernetes:the forceaddr can be addr in the endpoints
func WithForceAddr(ctx context.Context, forceaddr string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, forceaddrkey{}, forceaddr)
}
