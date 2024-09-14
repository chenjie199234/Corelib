package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

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
	self           string
	global         []OutsideHandler
	handler        map[string][]OutsideHandler
	handlerTimeout map[string]time.Duration
	instance       *stream.Instance
	stop           *graceful.Graceful
}
type client struct {
	sync.RWMutex
	stop bool
	ctxs map[uint64]*ServerContext
}

// if tlsc is not nil,the tls will be actived
func NewCrpcServer(c *ServerConfig, selfproject, selfgroup, selfapp string, tlsc *tls.Config) (*CrpcServer, error) {
	//pre check
	selffullname, e := name.MakeFullName(selfproject, selfgroup, selfapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	serverinstance := &CrpcServer{
		c:              c,
		tlsc:           tlsc,
		self:           selffullname,
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
		s.tellAllPeerSelfClosed()
		s.instance.Stop()
	} else {
		s.stop.Close(s.tellAllPeerSelfClosed, s.instance.Stop)
	}
}
func (s *CrpcServer) tellAllPeerSelfClosed() {
	s.instance.RangePeers(true, func(p *stream.Peer) {
		if tmpdata := p.GetData(); tmpdata != nil {
			c := (*client)(tmpdata)
			c.Lock()
			c.stop = true
			c.Unlock()
			d, _ := proto.Marshal(&Msg{
				B: &MsgBody{Error: cerror.ErrServerClosing},
			})
			p.SendMessage(nil, d, nil, nil)
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
	if common.BTS(peerVerifyData) != s.self {
		return nil, "", false
	}
	return nil, "", true
}

// return false will close the connection
func (s *CrpcServer) onlinefunc(ctx context.Context, p *stream.Peer) bool {
	if s.stop.Closing() {
		//tel peer self closed
		d, _ := proto.Marshal(&Msg{
			B: &MsgBody{Error: cerror.ErrServerClosing},
		})
		p.SendMessage(nil, d, nil, nil)
	}
	c := &client{
		stop: false,
		ctxs: make(map[uint64]*ServerContext),
	}
	p.SetData(unsafe.Pointer(c))
	slog.InfoContext(nil, "[crpc.server] online", slog.String("cip", p.GetRealPeerIP()))
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		slog.ErrorContext(nil, "[crpc.server] userdata format wrong", slog.String("cip", p.GetRealPeerIP()))
		p.Close(false)
		return
	}
	c := (*client)(p.GetData())
	switch msg.H.Type {
	case MsgType_Init:
		c.Lock()
		if c.stop {
			c.Unlock()
			//tell peer self closed
			msg.B.Body = nil
			msg.B.Error = cerror.ErrServerClosing
			msg.B.Traildata = nil
			msg.H.Metadata = nil
			msg.H.Tracedata = nil
			msg.H.Deadline = 0
			msg.H.Type = MsgType_Send
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(nil, d, nil, nil); e != nil {
				if e == stream.ErrConnClosed {
					e = cerror.ErrClosed
				} else if e == stream.ErrMsgLarge {
					//this is impossible
					e = cerror.ErrRespmsgLen
				}
				slog.ErrorContext(nil, "[crpc.server] write response failed",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.H.Path),
					slog.String("error", e.Error()))
			}
			return
		}
		if _, ok := c.ctxs[msg.H.Callid]; ok {
			//this is impossible
			c.Unlock()
			slog.ErrorContext(nil, "[crpc.server] duplicate init callid", slog.String("cip", p.GetRealPeerIP()), slog.String("path", msg.H.Path))
			p.Close(false)
			return
		}
		if e := s.stop.Add(1); e != nil {
			c.Unlock()
			msg.B.Body = nil
			if e == graceful.ErrClosing {
				//tell peer self closed
				msg.B.Error = cerror.ErrServerClosing
				msg.B.Traildata = nil
			} else {
				//tell peer self busy
				msg.B.Error = cerror.ErrBusy
				msg.B.Traildata = map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
			}
			msg.H.Metadata = nil
			msg.H.Tracedata = nil
			msg.H.Deadline = 0
			msg.H.Type = MsgType_Send
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(nil, d, nil, nil); e != nil {
				if e == stream.ErrConnClosed {
					e = cerror.ErrClosed
				} else if e == stream.ErrMsgLarge {
					//this is impossible
					e = cerror.ErrRespmsgLen
				}
				slog.ErrorContext(nil, "[crpc.server] write response failed",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.H.Path),
					slog.String("error", e.Error()))
			}
			return
		}

		handlers, ok := s.handler[msg.H.Path]
		if !ok {
			c.Unlock()
			slog.ErrorContext(nil, "[crpc.server] path doesn't exist", slog.String("cip", p.GetRealPeerIP()), slog.String("path", msg.H.Path))
			msg.B.Body = nil
			msg.B.Traildata = map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
			msg.B.Error = cerror.ErrNoapi
			msg.H.Metadata = nil
			msg.H.Tracedata = nil
			msg.H.Deadline = 0
			msg.H.Type = MsgType_Send
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(nil, d, nil, nil); e != nil {
				if e == stream.ErrConnClosed {
					e = cerror.ErrClosed
				} else if e == stream.ErrMsgLarge {
					//this is impossible
					e = cerror.ErrRespmsgLen
				}
				slog.ErrorContext(nil, "[crpc.server] write response failed",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.H.Path),
					slog.String("error", e.Error()))
			}
			return
		}

		//deal trace data
		peerip := p.GetRealPeerIP()
		var basectx context.Context
		var span *trace.Span
		if len(msg.H.Tracedata) == 0 || msg.H.Tracedata["TraceID"] == "" || msg.H.Tracedata["SpanID"] == "" {
			basectx, span = trace.NewSpan(p, "Corelib.Crpc", trace.Server, nil)
			span.GetParentSpanData().SetStateKV("app", "unknown")
			span.GetParentSpanData().SetStateKV("host", p.GetRealPeerIP())
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		} else {
			tid, e := trace.TraceIDFromHex(msg.H.Tracedata["TraceID"])
			if e != nil {
				c.Unlock()
				slog.ErrorContext(nil, "[crpc.server] trace data fromat wrong",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.H.Path),
					slog.String("trace_id", msg.H.Tracedata["TraceID"]))
				p.Close(false)
				return
			}
			psid, e := trace.SpanIDFromHex(msg.H.Tracedata["SpanID"])
			if e != nil {
				c.Unlock()
				slog.ErrorContext(nil, "[crpc.server] trace data fromat wrong",
					slog.String("cip", p.GetRealPeerIP()),
					slog.String("path", msg.H.Path),
					slog.String("p_span_id", msg.H.Tracedata["SpanID"]))
				p.Close(false)
				return
			}
			parent := trace.NewSpanData(tid, psid)
			var app, host, method, path bool
			for k, v := range msg.H.Tracedata {
				if k == "TraceID" || k == "SpanID" {
					continue
				}
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
				parent.SetStateKV("host", p.GetRealPeerIP())
			}
			if !method {
				parent.SetStateKV("method", "unknown")
			}
			if !path {
				parent.SetStateKV("path", "unknown")
			}
			basectx, span = trace.NewSpan(p, "Corelib.Crpc", trace.Server, parent)
		}
		span.GetSelfSpanData().SetStateKV("app", s.self)
		span.GetSelfSpanData().SetStateKV("host", host.Hostip)
		span.GetSelfSpanData().SetStateKV("method", "CRPC")
		span.GetSelfSpanData().SetStateKV("path", msg.H.Path)

		//deal metadata
		if msg.H.Metadata == nil {
			msg.H.Metadata = map[string]string{"Client-IP": peerip}
		} else if _, ok := msg.H.Metadata["Client-IP"]; !ok {
			msg.H.Metadata["Client-IP"] = peerip
		}
		basectx = metadata.SetMetadata(basectx, msg.H.Metadata)

		//deal timeout
		var basecancel context.CancelFunc
		if servertimeout := int64(s.getHandlerTimeout(msg.H.Path)); servertimeout > 0 {
			serverdl := time.Now().UnixNano() + servertimeout
			if msg.H.Deadline != 0 {
				//compare use the small one
				if msg.H.Deadline < serverdl {
					basectx, basecancel = context.WithDeadline(basectx, time.Unix(0, msg.H.Deadline))
				} else {
					basectx, basecancel = context.WithDeadline(basectx, time.Unix(0, serverdl))
				}
			} else {
				//use server timeout
				basectx, basecancel = context.WithDeadline(basectx, time.Unix(0, serverdl))
			}
		} else if msg.H.Deadline != 0 {
			//use client timeout
			basectx, basecancel = context.WithDeadline(basectx, time.Unix(0, msg.H.Deadline))
		} else {
			//no timeout
			basectx, basecancel = context.WithCancel(basectx)
		}

		//make workctx
		rw := newrw(msg.H.Callid, msg.H.Path, 0, nil, nil, func(ctx context.Context, m *Msg) error {
			d, _ := proto.Marshal(m)
			e := p.SendMessage(ctx, d, nil, nil)
			if e != nil {
				if e == stream.ErrMsgLarge {
					e = cerror.ErrRespmsgLen
				} else if e == stream.ErrConnClosed {
					e = cerror.ErrClosed
				} else if e == context.DeadlineExceeded {
					e = cerror.ErrDeadlineExceeded
				} else if e == context.Canceled {
					e = cerror.ErrCanceled
				} else {
					//this is impossible
					e = cerror.Convert(e)
				}
			}
			return e
		})
		if msg.WithB {
			rw.cache(msg.B)
		}
		workctx := &ServerContext{
			Context:    basectx,
			CancelFunc: basecancel,
			rw:         rw,
			peer:       p,
			peerip:     peerip,
		}
		c.ctxs[msg.H.Callid] = workctx
		c.Unlock()
		go func() {
			defer func() {
				if e := recover(); e != nil {
					stack := make([]byte, 1024)
					n := runtime.Stack(stack, false)
					slog.ErrorContext(workctx, "[crpc.server] panic",
						slog.String("cip", peerip),
						slog.String("path", msg.H.Path),
						slog.Any("panic", e),
						slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
					workctx.StopSend(cerror.ErrPanic)
				}
				span.Finish(workctx.e)
				peername, _ := span.GetParentSpanData().GetStateKV("app")
				monitor.CrpcServerMonitor(peername, "CRPC", msg.H.Path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
				s.stop.DoneOne()
			}()
			for _, handler := range handlers {
				handler(workctx)
				if workctx.finish {
					break
				}
			}
			c.Lock()
			if ctx, ok := c.ctxs[msg.H.Callid]; ok {
				ctx.CancelFunc()
				delete(c.ctxs, msg.H.Callid)
			}
			c.Unlock()
			rw.closereadwrite()
		}()
	case MsgType_Send:
		c.RLock()
		if ctx, ok := c.ctxs[msg.H.Callid]; ok {
			ctx.rw.cache(msg.B)
		}
		c.RUnlock()
	case MsgType_Cancel:
		c.RLock()
		if ctx, ok := c.ctxs[msg.H.Callid]; ok {
			ctx.CancelFunc()
		}
		c.RUnlock()
	case MsgType_CloseRead:
		c.RLock()
		if ctx, ok := c.ctxs[msg.H.Callid]; ok {
			atomic.AndInt32(&ctx.rw.status, 0b0111)
		}
		c.RUnlock()
	case MsgType_CloseSend:
		c.RLock()
		if ctx, ok := c.ctxs[msg.H.Callid]; ok {
			atomic.AndInt32(&ctx.rw.status, 0b1011)
			ctx.rw.reader.Close()
		}
		c.RUnlock()
	case MsgType_CloseReadSend:
		c.RLock()
		if ctx, ok := c.ctxs[msg.H.Callid]; ok {
			atomic.AndInt32(&ctx.rw.status, 0b0111)
			atomic.AndInt32(&ctx.rw.status, 0b1011)
			ctx.rw.reader.Close()
		}
		c.RUnlock()
	}
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	slog.InfoContext(nil, "[crpc.server] offline", slog.String("cip", p.GetRealPeerIP()))
}
