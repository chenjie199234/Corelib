package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
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

type OutsideHandler func(*ServerContext)

type ServerConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the GlobalTimeout
	//the client's deadline will also effect the rpc call's final deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout,if >0 min is HeartProbe
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

type CrpcServer struct {
	c              *ServerConfig
	tlsc           *tls.Config
	global         []OutsideHandler
	handler        map[string][]OutsideHandler
	handlerTimeout map[string]time.Duration
	instance       *stream.Instance
	stop           *graceful.Graceful
}
type client struct {
	sync.RWMutex
	ctxs map[uint64]*ServerContext
}

// if tlsc is not nil,the tls will be actived
func NewCrpcServer(c *ServerConfig, tlsc *tls.Config) (*CrpcServer, error) {
	if e := cotel.Init(); e != nil {
		return nil, e
	}
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[crpc.server] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	if c == nil {
		c = &ServerConfig{}
	}
	serverinstance := &CrpcServer{
		c:              c,
		tlsc:           tlsc,
		global:         make([]OutsideHandler, 0, 10),
		handler:        make(map[string][]OutsideHandler, 10),
		handlerTimeout: make(map[string]time.Duration),
		stop:           graceful.New(),
	}
	instancec := &stream.InstanceConfig{
		RecvIdleTimeout:    c.IdleTimeout.StdDuration(),
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout.StdDuration(),
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	instancec.VerifyFunc = serverinstance.verifyfunc
	instancec.OnlineFunc = serverinstance.onlinefunc
	instancec.UserdataFunc = serverinstance.userfunc
	instancec.OfflineFunc = serverinstance.offlinefunc
	serverinstance.instance, _ = stream.NewInstance(instancec)
	return serverinstance, nil
}

var ErrServerClosed = errors.New("[crpc.server] closed")

func (s *CrpcServer) StartCrpcServer(listenaddr string) error {
	e := s.instance.StartServer(listenaddr, s.tlsc)
	if e == stream.ErrServerClosed {
		return ErrServerClosed
	}
	return e
}
func (s *CrpcServer) GetClientNum() int32 {
	return s.instance.GetPeerNum()
}
func (s *CrpcServer) GetReqNum() int64 {
	return s.stop.GetNum()
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (s *CrpcServer) StopCrpcServer(force bool) {
	s.instance.PreStop()
	if force {
		s.stop.ForceClose(func() {
			s.tellAllPeerSelfClosed()
			s.instance.Stop()
		})
	} else {
		s.stop.Close(func() {
			s.instance.PreStop()
			s.tellAllPeerSelfClosed()
		}, s.instance.Stop)
	}
}
func (s *CrpcServer) tellAllPeerSelfClosed() {
	s.instance.RangePeers(true, func(p *stream.Peer) {
		if tmp := p.GetData(); tmp != nil {
			wg := sync.WaitGroup{}
			m := &Msg{}
			mh := &Msg_Header{}
			mh.SetCallid(0)
			mh.SetType(MsgType_SEND)
			mb := &Msg_Body{}
			mb.SetError(cerror.ErrServerClosing)
			m.SetH(mh)
			m.SetB(mb)
			d, _ := proto.Marshal(m)
			wg.Go(func() {
				p.SendMessage(context.Background(), d, nil, nil)
			})
			wg.Wait()
		}
	})
}

// first key path,second key method,value timeout(if timeout <= 0 means no timeout)
func (this *CrpcServer) UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmp := make(map[string]time.Duration)
	for path := range timeout {
		for method, to := range timeout[path] {
			if strings.ToUpper(method) != "CRPC" {
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

func (this *CrpcServer) getHandlerTimeout(path string) time.Duration {
	if t, ok := this.handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout.StdDuration()
}

func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

func (s *CrpcServer) RegisterHandler(sname, mname string, handlers ...OutsideHandler) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	s.handler[path] = totalhandlers
}

// return false will close the connection
func (s *CrpcServer) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, string, bool) {
	if s.stop.Closing() {
		//self closing
		return nil, "", false
	}
	if common.BTS(peerVerifyData) != name.GetSelfFullName() {
		return nil, "", false
	}
	return nil, "", true
}

// return false will close the connection
func (s *CrpcServer) onlinefunc(ctx context.Context, p *stream.Peer) bool {
	if s.stop.Closing() {
		//tel peer self closed
		m := &Msg{}
		mh := &Msg_Header{}
		mh.SetCallid(0)
		mh.SetType(MsgType_SEND)
		mb := &Msg_Body{}
		mb.SetError(cerror.ErrServerClosing)
		m.SetH(mh)
		m.SetB(mb)
		d, _ := proto.Marshal(m)
		p.SendMessage(context.Background(), d, nil, nil)
	}
	c := &client{
		ctxs: make(map[uint64]*ServerContext),
	}
	p.SetData(unsafe.Pointer(c))
	slog.Info("[crpc.server] online", slog.String("cip", p.GetRealPeerIP()))
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		slog.Error("[crpc.server] userdata format wrong", slog.String("cip", p.GetRealPeerIP()))
		p.Close(false)
		return
	}
	c := (*client)(p.GetData())
	switch msg.GetH().GetType() {
	case MsgType_INIT:
		c.RLock()
		_, ok := c.ctxs[msg.GetH().GetCallid()]
		c.RUnlock()
		if ok {
			p.Close(false)
			slog.Error("[crpc.server] duplicate init callid",
				slog.String("cip", p.GetRealPeerIP()),
				slog.String("path", msg.GetH().GetPath()))
			return
		}
		if e := s.stop.Add(1); e != nil {
			if e == graceful.ErrClosing {
				//tell peer self closed
				mb := &Msg_Body{}
				mb.SetError(cerror.ErrServerClosing)
				msg.SetB(mb)
			} else {
				//tell peer self busy,this is impossible
				mb := &Msg_Body{}
				mb.SetError(cerror.ErrBusy)
				msg.SetB(mb)
			}
			msg.GetH().SetMetadata(nil)
			msg.GetH().SetTracedata(nil)
			msg.GetH().ClearDeadline()
			msg.GetH().SetType(MsgType_SEND)
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(context.Background(), d, nil, nil); e != nil {
				switch e {
				case stream.ErrConnClosed:
					e = cerror.ErrClosed
				case stream.ErrMsgLarge:
					//this is impossible
					e = cerror.ErrRespmsgLen
				}
				slog.Error("[crpc.server] write response failed",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.GetH().GetPath()),
					slog.String("error", e.Error()))
			}
			return
		}
		handlers, ok := s.handler[msg.GetH().GetPath()]
		if !ok {
			slog.Error("[crpc.server] path doesn't exist",
				slog.String("cip", p.GetRealPeerIP()), slog.String("path", msg.GetH().GetPath()))
			mb := &Msg_Body{}
			mb.SetError(cerror.ErrNoapi)
			msg.SetB(mb)
			msg.GetH().SetMetadata(nil)
			msg.GetH().SetTracedata(nil)
			msg.GetH().ClearDeadline()
			msg.GetH().SetType(MsgType_SEND)
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(context.Background(), d, nil, nil); e != nil {
				switch e {
				case stream.ErrConnClosed:
					e = cerror.ErrClosed
				case stream.ErrMsgLarge:
					//this is impossible
					e = cerror.ErrRespmsgLen
				}
				slog.Error("[crpc.server] write response failed",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.GetH().GetPath()),
					slog.String("error", e.Error()))
			}
			s.stop.DoneOne()
			return
		}
		//response init success
		r := &Msg{}
		rh := &Msg_Header{}
		rh.SetCallid(msg.GetH().GetCallid())
		rh.SetType(MsgType_INIT_SUCCESS)
		r.SetH(rh)
		rd, _ := proto.Marshal(r)
		if e := p.SendMessage(context.Background(), rd, nil, nil); e != nil {
			switch e {
			case stream.ErrConnClosed:
				e = cerror.ErrClosed
			case stream.ErrMsgLarge:
				//this is impossible
				e = cerror.ErrRespmsgLen
			}
			slog.Error("[crpc.server] write response failed",
				slog.String("cip", p.GetRealPeerIP()),
				slog.String("path", msg.GetH().GetPath()),
				slog.String("error", e.Error()))
			s.stop.DoneOne()
			return
		}
		peerip := p.GetRealPeerIP()
		//trace
		clientname := msg.GetH().GetTracedata()["Core-Self"]
		if clientname == "" {
			clientname = "unknown"
		}
		basectx, span := otel.Tracer("Corelib.crpc.server", trace.WithInstrumentationVersion(version.String())).Start(
			otel.GetTextMapPropagator().Extract(p, propagation.MapCarrier(msg.GetH().GetTracedata())),
			"handle crpc",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("url.path", msg.GetH().GetPath()), attribute.String("client.name", clientname), attribute.String("client.ip", peerip)))
		//metadata
		if msg.GetH().GetMetadata() == nil {
			msg.GetH().SetMetadata(map[string]string{"Client-IP": peerip})
		} else if _, ok := msg.GetH().GetMetadata()["Client-IP"]; !ok {
			msg.GetH().GetMetadata()["Client-IP"] = peerip
		}
		basectx = metadata.SetMetadata(basectx, msg.GetH().GetMetadata())

		//deadline
		var dl time.Time
		var basecancel context.CancelFunc
		if sto := s.getHandlerTimeout(msg.GetH().GetPath()); sto > 0 {
			//use server timeout
			dl = time.Now().Add(sto)
			if msg.GetH().GetDeadline() != 0 && msg.GetH().GetDeadline() < dl.UnixNano() {
				//use client deadline
				dl = time.Unix(0, msg.GetH().GetDeadline())
			}
		} else if msg.GetH().GetDeadline() != 0 {
			//use client deadline
			dl = time.Unix(0, msg.GetH().GetDeadline())
		}
		if dl.IsZero() {
			//no timeout
			basectx, basecancel = context.WithCancel(basectx)
		} else {
			basectx, basecancel = context.WithDeadline(basectx, dl)
		}

		//make workctx
		rw := newrw(msg.GetH().GetCallid(), msg.GetH().GetPath(), 0, nil, nil, func(ctx context.Context, m *Msg) error {
			d, _ := proto.Marshal(m)
			e := p.SendMessage(ctx, d, nil, nil)
			switch e {
			case stream.ErrMsgLarge:
				e = cerror.ErrRespmsgLen
			case stream.ErrConnClosed:
				e = cerror.ErrClosed
			case context.DeadlineExceeded:
				e = cerror.ErrDeadlineExceeded
			case context.Canceled:
				e = cerror.ErrCanceled
			case nil:
			default:
				//this is impossible
				e = cerror.Convert(e)
			}
			return e
		})
		workctx := &ServerContext{
			Context: basectx,
			cancel:  basecancel,
			rw:      rw,
			peer:    p,
			peerip:  peerip,
		}
		c.Lock()
		c.ctxs[msg.GetH().GetCallid()] = workctx
		c.Unlock()

		var tmer *time.Timer
		if !dl.IsZero() {
			tmer = time.AfterFunc(time.Until(dl), func() {
				workctx.closed.Store(true)
				mb := &Msg_Body{}
				mb.SetError(cerror.ErrDeadlineExceeded)
				//the error response shouldn't send failed due to the Context's error
				//the error response can only send failed when the connection closed
				workctx.rw.send(context.Background(), mb)
				workctx.rw.closerecv()
			})
		}
		go func() {
			defer func() {
				if e := recover(); e != nil {
					stack := make([]byte, 1024)
					n := runtime.Stack(stack, false)
					slog.ErrorContext(workctx, "[crpc.server] panic",
						slog.String("cip", peerip),
						slog.String("path", msg.GetH().GetPath()),
						slog.Any("panic", e),
						slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
					workctx.Abort(cerror.ErrPanic)
				}

				workctx.cancel()

				if tmer != nil {
					tmer.Stop()
				}

				c.Lock()
				delete(c.ctxs, msg.GetH().GetCallid())
				c.Unlock()

				if workctx.e != nil {
					span.SetStatus(codes.Error, workctx.e.Error())
				} else {
					span.SetStatus(codes.Ok, "")
				}
				span.End()
				if ros, ok := span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
					mstatus, _ := otel.Meter("Corelib.crpc.server", metric.WithInstrumentationVersion(version.String())).Int64Histogram(msg.GetH().GetPath()+".status", metric.WithUnit("1"), metric.WithExplicitBucketBoundaries(0))
					if workctx.e != nil {
						mstatus.Record(context.Background(), 1)
					} else {
						mstatus.Record(context.Background(), 0)
					}
					mtime, _ := otel.Meter("Corelib.crpc.server", metric.WithInstrumentationVersion(version.String())).Float64Histogram(msg.GetH().GetPath()+".time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
					mtime.Record(context.Background(), float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0)
				}
				s.stop.DoneOne()
			}()
			for _, handler := range handlers {
				handler(workctx)
				if workctx.closed.Load() {
					break
				}
			}
			if !workctx.closed.Swap(true) {
				rw.closerecvsend()
			}
		}()
	case MsgType_SEND:
		c.RLock()
		ctx, ok := c.ctxs[msg.GetH().GetCallid()]
		c.RUnlock()
		if ok {
			if ctx.rw.status.Load()&0b00100 != 0 {
				ctx.rw.cache(msg.GetB())
				if ctx.rw.status.Load()&0b01100 == 0 {
					//same as MsgType_CLOSE_RECV_SEND
					ctx.cancel()
					c.Lock()
					delete(c.ctxs, msg.GetH().GetCallid())
					c.Unlock()
				}
			} else {
				//ignore the message after peer stopsend
			}
		}
	case MsgType_CLOSE_RECV:
		c.RLock()
		ctx, ok := c.ctxs[msg.GetH().GetCallid()]
		c.RUnlock()
		if ok {
			old := ctx.rw.status.And(0b10111)
			if (old&0b10111)&0b01100 == 0 {
				//same as MsgType_CLOSE_RECV_SEND
				ctx.cancel()
				c.Lock()
				delete(c.ctxs, msg.GetH().GetCallid())
				c.Unlock()
			}
		}
	case MsgType_CLOSE_SEND:
		c.RLock()
		ctx, ok := c.ctxs[msg.GetH().GetCallid()]
		c.RUnlock()
		if ok {
			old := ctx.rw.status.And(0b11011)
			ctx.rw.reader.Close()
			if (old&0b11011)&0b01100 == 0 {
				//same as MsgType_CLOSE_RECV_SEND
				ctx.cancel()
				c.Lock()
				delete(c.ctxs, msg.GetH().GetCallid())
				c.Unlock()
			}
		}
	case MsgType_CLOSE_RECV_SEND:
		c.RLock()
		ctx, ok := c.ctxs[msg.GetH().GetCallid()]
		c.RUnlock()
		if ok {
			ctx.rw.status.And(0b10011)
			ctx.rw.reader.Close()
			ctx.cancel()
			c.Lock()
			delete(c.ctxs, msg.GetH().GetCallid())
			c.Unlock()
		}
	}
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	c := (*client)(p.GetData())
	c.Lock()
	for _, ctx := range c.ctxs {
		ctx.rw.status.And(0b00011)
		ctx.rw.reader.Close()
		ctx.cancel()
	}
	c.ctxs = nil
	c.Unlock()
	slog.Info("[crpc.server] offline", slog.String("cip", p.GetRealPeerIP()))
}
