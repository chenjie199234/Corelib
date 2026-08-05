package crpc

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/container/list"
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
	//if ctx's Deadline exist and DefaultHandlerTimeout > 0,the min(time.Now().Add(DefaultHandlerTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and DefaultHandlerTimeout > 0 ,the time.Now().Add(DefaultHandlerTimeout) will be used as the final deadline
	//if ctx's deadline not exist and DefaultHandlerTimeout <=0,means no deadline
	DefaultHandlerTimeout ctime.Duration `json:"default_handler_timeout"`
	//time for connection establich(include dial time,tls handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//time for read one complete message,start from read the first byte of the message
	//<=0 means no timeout
	ReadTimeout ctime.Duration `json:"read_timeout"`
	//time for write one piece of the complete message(big message will split to many pieces to send,one piece's max len is 65536)
	//the connection's write deadline will be resetted every time start to write a piece of data
	//<=0 means no timeout
	WriteTimeout ctime.Duration `json:"write_timeout"`
	//time for waiting the next message,it will be reset when starting to read next message
	//expired without next message,the connection will be closed
	//<=0 means no timeout
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

	statusCounter metric.Int64Counter
	timeHistogram metric.Float64Histogram
	tracer        trace.Tracer

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be activated
func NewCrpcClient(c *ClientConfig, d discover.DI, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*CrpcClient, error) {
	if e := cotel.Init(); e != nil {
		return nil, e
	}
	sCounter, e := otel.Meter("Corelib.crpc.client", metric.WithInstrumentationVersion(version.String())).Int64Counter("crpc.client.path.status", metric.WithUnit("1"))
	if e != nil {
		return nil, e
	}
	tHistogram, e := otel.Meter("Corelib.crpc.client", metric.WithInstrumentationVersion(version.String())).Float64Histogram("crpc.client.path.time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	if e != nil {
		return nil, e
	}
	if tlsc != nil {
		tlsc = tlsc.Clone()
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

		statusCounter: sCounter,
		timeHistogram: tHistogram,
		tracer:        otel.Tracer("Corelib.crpc.client", trace.WithInstrumentationVersion(version.String())),

		stop: graceful.New(),
	}
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		ConnectTimeout:     c.ConnectTimeout.StdDuration(),
		ReadTimeout:        c.ReadTimeout.StdDuration(),
		WriteTimeout:       c.WriteTimeout.StdDuration(),
		IdleTimeout:        c.IdleTimeout.StdDuration(),
		MaxMsgLen:          c.MaxMsgLen,
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
		c.stop.ForceClose(func() {
			c.resolver.Close()
			c.instance.Stop()
		})
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
	server.closing.Store(false)
	c.balancer.RebuildPicker(server.addr, true)
	slog.Info("[crpc.client] online", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		slog.Error("[crpc.client] userdata format wrong", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
		return
	}
	switch msg.GetH().GetType() {
	case MsgType_INIT_SUCCESS:
		if rw := server.getrw(msg.GetH().GetCallid()); rw != nil {
			rw.cache(nil)
		}
	case MsgType_CLOSE_RECV:
		if rw := server.getrw(msg.GetH().GetCallid()); rw != nil {
			old := rw.status.And(0b10111)
			if (old&0b10111)&0b01100 == 0 {
				//same as MsgType_CLOSE_RECV_SEND
				server.delrw(msg.GetH().GetCallid())
			}
		}
	case MsgType_CLOSE_SEND:
		if rw := server.getrw(msg.GetH().GetCallid()); rw != nil {
			old := rw.status.And(0b11011)
			rw.reader.Close()
			if (old&0b11011)&0b01100 == 0 {
				//same as MsgType_CLOSE_RECV_SEND
				server.delrw(msg.GetH().GetCallid())
			}
		}
	case MsgType_CLOSE_RECV_SEND:
		if rw := server.getrw(msg.GetH().GetCallid()); rw != nil {
			rw.status.And(0b10011)
			rw.reader.Close()
			server.delrw(msg.GetH().GetCallid())
		}
	case MsgType_SEND:
		if msg.GetB().GetError() != nil && cerror.Equal(msg.GetB().GetError(), cerror.ErrServerClosing) {
			if !server.closing.Swap(true) {
				//set the lowest pick priority
				server.Pickinfo.SetDiscoverServerOffline(0)
				//rebuild picker
				c.balancer.RebuildPicker(server.addr, false)
				//triger discover
				c.resolver.Now()
			}
		}
		if msg.GetH().GetCallid() == 0 {
			return
		}
		rw := server.getrw(msg.GetH().GetCallid())
		if rw == nil {
			return
		}
		if rw.status.Load()&0b00100 != 0 {
			rw.cache(msg.GetB())
			if rw.status.Load()&0b01100 == 0 {
				//same as MsgType_CLOSE_RECV_SEND
				server.delrw(rw.callid)
			}
		} else {
			//ignore the message after peer stopsend
		}
	}
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	slog.Info("[crpc.client] offline", slog.String("sname", c.serverfullname), slog.String("sip", server.addr))
	server.setpeer(nil)
	c.balancer.RebuildPicker(server.addr, false)
	server.cleanrw()
	go c.start(server, true)
}

// 'in' and 'encoder' are a pair.the 'encoder' describes how the 'in' data be encoded
// if 'encoder' is not Encoder_UNKNOWN,means,this call will only send once with the data 'in' and all the ctx.Send in handler will fail
func (c *CrpcClient) Call(ctx context.Context, path string, in []byte, encoder Encoder, handler func(ctx *CallContext) error) error {
	if _, ok := Encoder_name[int32(encoder)]; !ok {
		return cerror.ErrReq
	}
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return cerror.ErrClientClosing
		}
		return cerror.ErrBusy
	}
	defer c.stop.DoneOne()

	cancelOutSide := ctx.Done() != nil

	if c.c.DefaultHandlerTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.DefaultHandlerTimeout.StdDuration()))
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
		tctx, span := c.tracer.Start(ctx, "call crpc", trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("path", path), attribute.String("sname", c.serverfullname)))
		otel.GetTextMapPropagator().Inject(tctx, propagation.MapCarrier(td))
		var stime int64 //used in metric and balancer
		if cotel.NeedTrace() {
			stime = span.(sdktrace.ReadOnlySpan).StartTime().UnixNano()
		} else {
			stime = time.Now().UnixNano()
		}
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			slog.ErrorContext(ctx, "[crpc.client] pick server failed",
				slog.String("sname", c.serverfullname),
				slog.String("path", path),
				slog.String("error", e.Error()))
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			return e
		}
		span.SetAttributes(attribute.String("sip", server.addr))
		rw := server.createrw(path, deadline, md, td)
		if e := rw.init(ctx, nil); e != nil {
			server.delrw(rw.callid)
			slog.ErrorContext(ctx, "[crpc.client] send init header failed",
				slog.String("sname", c.serverfullname),
				slog.String("sip", server.addr),
				slog.String("path", path),
				slog.String("error", e.Error()))
			span.SetStatus(codes.Error, e.Error())
			span.End()
			server.GetServerPickInfo().Done(false, 0)
			if cerror.Equal(e, cerror.ErrClosed) {
				//send failed,the server will not get the init header,we can retry this request
				continue
			}
			//only record the real call's metric
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			return e
		}
		//check the first msg returned from server
		//it should be INIT_SUCCESS or error
		mb, e := rw.reader.Pop(ctx)
		switch e {
		case context.Canceled:
			rw.closerecvsend()
			e = cerror.ErrCanceled
		case context.DeadlineExceeded:
			rw.closerecvsend()
			e = cerror.ErrDeadlineExceeded
		case list.ErrClosed:
			//the reader is closed,only happened when the connection is closed and the offlinefunc is called
			e = cerror.ErrClosed
		default:
			if mb != nil {
				//the first msg returned from server is not INIT_SUCCESS,must be error
				e = mb.GetError()
			}
			//first msg returned from server is INIT_SUCCESS
		}
		if e != nil {
			server.delrw(rw.callid)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			server.GetServerPickInfo().Done(false, 0)
			if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrClosed) {
				//the server is closing or already closed,our init request was ignored,we can retry safely
				continue
			}
			//only record the real call's metric
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			return e
		}
		if _, ok := Encoder_name[int32(encoder)]; ok && encoder != Encoder_UNKNOWN {
			mb := &Msg_Body{}
			mb.SetBody(in)
			mb.SetBodyEncoder(encoder)
			if e := rw.send(ctx, mb); e != nil {
				server.delrw(rw.callid)
				slog.ErrorContext(ctx, "[crpc.client] send init body failed",
					slog.String("sname", c.serverfullname),
					slog.String("sip", server.addr),
					slog.String("path", path),
					slog.String("error", e.Error()))
				span.SetStatus(codes.Error, e.Error())
				span.End()
				server.GetServerPickInfo().Done(false, 0)
				if cerror.Equal(e, cerror.ErrClosed) {
					//send failed,the server will not get the init body,we can retry this request
					continue
				}
				if cotel.NeedMetric() {
					var etime int64
					if cotel.NeedTrace() {
						etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
					} else {
						etime = time.Now().UnixNano()
					}
					c.recordmetric(path, float64(etime-stime)/1000000.0, true)
				}
				return e
			}
			//Call with Body,means,this call will only send data once and the data is sended by init
			rw.closesend()
		}
		var stopch chan struct{}
		var tmer *time.Timer
		if cancelOutSide {
			//the context can be canceled outside,we need to catch the cancel in goroutine
			stopch = make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					//deadline or canceled
				case <-stopch:
				}
				rw.closerecvsend()
			}()
		} else if deadline > 0 {
			//the context can't be canceled outside,but it has a deadline,we can catch it in timer
			//timer's goroutine is created when deadline arrived,and the timer can be stopped
			//we can save some computer resource by using timer
			tmer = time.AfterFunc(time.Until(time.Unix(0, deadline)), func() {
				rw.closerecvsend()
			})
		}
		e = handler(&CallContext{
			Context: ctx,
			rw:      rw,
			s:       server,
		})
		server.delrw(rw.callid)
		if cancelOutSide {
			close(stopch)
		} else if deadline > 0 {
			if tmer.Stop() {
				rw.closerecvsend()
			}
		} else {
			rw.closerecvsend()
		}
		if rw.peererror != nil {
			span.SetStatus(codes.Error, rw.peererror.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		var etime int64
		if cotel.NeedTrace() {
			etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
		} else {
			etime = time.Now().UnixNano()
		}
		server.GetServerPickInfo().Done(rw.peererror == nil, uint64(etime-stime))
		if cotel.NeedMetric() {
			c.recordmetric(path, float64(etime-stime)/1000000.0, rw.peererror != nil)
		}
		return e
	}
}

func (c *CrpcClient) recordmetric(path string, usetimems float64, err bool) {
	attr := attribute.String("path", path)
	if err {
		c.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "error")))
	} else {
		c.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "ok")))
	}
	c.timeHistogram.Record(context.Background(), usetimems, metric.WithAttributes(attr))
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
