package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(*Context)

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
	ctxpool        *sync.Pool
	handler        map[string]func(context.Context, *stream.Peer, *Msg)
	handlerTimeout map[string]time.Duration
	instance       *stream.Instance
	stop           *graceful.Graceful
}
type client struct {
	sync.RWMutex
	maxcallid uint64 //0-server is closing
	calls     map[uint64]context.CancelFunc
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
		ctxpool:        &sync.Pool{},
		handler:        make(map[string]func(context.Context, *stream.Peer, *Msg), 10),
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
		//tel all peers self closed
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
			d, _ := proto.Marshal(&Msg{
				Callid: c.maxcallid + 1,
				Error:  cerror.ErrServerClosing,
			})
			c.maxcallid = 0
			p.SendMessage(nil, d, nil, nil)
			c.Unlock()
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

// thread unsafe
func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *CrpcServer) RegisterHandler(sname, mname string, handlers ...OutsideHandler) {
	path := "/" + sname + "/" + mname
	s.handler[path] = s.insidehandler(path, handlers...)
}

func (s *CrpcServer) insidehandler(path string, handlers ...OutsideHandler) func(context.Context, *stream.Peer, *Msg) {
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(ctx context.Context, p *stream.Peer, msg *Msg) {
		peerip := p.GetRealPeerIP()
		var span *trace.Span
		if len(msg.Tracedata) == 0 || msg.Tracedata["TraceID"] == "" || msg.Tracedata["SpanID"] == "" {
			ctx, span = trace.NewSpan(ctx, "Corelib.Crpc", trace.Server, nil)
			span.GetParentSpanData().SetStateKV("app", "unknown")
			span.GetParentSpanData().SetStateKV("host", peerip)
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		} else {
			tid, e := trace.TraceIDFromHex(msg.Tracedata["TraceID"])
			if e != nil {
				log.Error(nil, "[crpc.server] trace data fromat wrong",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("trace_id", msg.Tracedata["TraceID"]))
				p.Close(false)
				return
			}
			psid, e := trace.SpanIDFromHex(msg.Tracedata["SpanID"])
			if e != nil {
				log.Error(nil, "[crpc.server] trace data fromat wrong",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("p_span_id", msg.Tracedata["SpanID"]))
				p.Close(false)
				return
			}
			parent := trace.NewSpanData(tid, psid)
			var app, host, method, path bool
			for k, v := range msg.Tracedata {
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
				parent.SetStateKV("host", peerip)
			}
			if !method {
				parent.SetStateKV("method", "unknown")
			}
			if !path {
				parent.SetStateKV("path", "unknown")
			}
			ctx, span = trace.NewSpan(ctx, "Corelib.Crpc", trace.Server, parent)
		}
		span.GetSelfSpanData().SetStateKV("app", s.self)
		span.GetSelfSpanData().SetStateKV("host", host.Hostip)
		span.GetSelfSpanData().SetStateKV("method", "CRPC")
		span.GetSelfSpanData().SetStateKV("path", path)

		if servertimeout := int64(s.getHandlerTimeout(path)); servertimeout > 0 {
			serverdl := time.Now().UnixNano() + servertimeout
			if msg.Deadline != 0 {
				//compare use the small one
				if msg.Deadline < serverdl {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Unix(0, msg.Deadline))
					defer cancel()
				} else {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Unix(0, serverdl))
					defer cancel()
				}
			} else {
				//use server timeout
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Unix(0, serverdl))
				defer cancel()
			}
		} else if msg.Deadline != 0 {
			//use client timeout
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, msg.Deadline))
			defer cancel()
		}
		if msg.Metadata == nil {
			msg.Metadata = map[string]string{"Client-IP": peerip}
		} else if _, ok := msg.Metadata["Client-IP"]; !ok {
			msg.Metadata["Client-IP"] = peerip
		}
		//logic
		workctx := s.getContext(metadata.SetMetadata(ctx, msg.Metadata), p, msg, peerip, totalhandlers)
		paniced := true
		defer func() {
			if paniced {
				e := recover()
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[crpc.server] panic",
					log.String("cip", peerip),
					log.String("path", path),
					log.Any("panic", e),
					log.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrPanic
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			msg.Metadata = map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(workctx, d, nil, nil); e != nil {
				if e == stream.ErrMsgLarge {
					log.Error(workctx, "[crpc.server] write response failed",
						log.String("cip", peerip),
						log.String("path", path),
						log.CError(cerror.ErrRespmsgLen))
					msg.Path = ""
					msg.Deadline = 0
					msg.Body = nil
					msg.Error = cerror.ErrRespmsgLen
					msg.Tracedata = nil
					d, _ = proto.Marshal(msg)
					if e = p.SendMessage(workctx, d, nil, nil); e != nil {
						if e == stream.ErrConnClosed {
							msg.Error = cerror.ErrClosed
						} else {
							msg.Error = cerror.ConvertStdError(e)
						}
						log.Error(workctx, "[crpc.server] write response failed",
							log.String("cip", peerip),
							log.String("path", path),
							log.CError(msg.Error))
					}
				} else if e == stream.ErrConnClosed {
					log.Error(workctx, "[crpc.server] write response failed",
						log.String("cip", peerip),
						log.String("path", path),
						log.CError(cerror.ErrClosed))
					msg.Error = cerror.ErrClosed
				} else {
					msg.Error = cerror.ConvertStdError(e)
					log.Error(workctx, "[crpc.server] write response failed",
						log.String("cip", peerip),
						log.String("path", path),
						log.CError(msg.Error))
				}
			}
			span.Finish(msg.Error)
			peername, _ := span.GetParentSpanData().GetStateKV("app")
			monitor.CrpcServerMonitor(peername, "CRPC", path, msg.Error, uint64(span.GetEnd()-span.GetStart()))
			s.putContext(workctx)
		}()
		workctx.run()
		paniced = false
	}
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
			Callid: 0,
			Error:  cerror.ErrServerClosing,
		})
		p.SendMessage(nil, d, nil, nil)
	}
	c := &client{
		maxcallid: 1,
		calls:     make(map[uint64]context.CancelFunc),
	}
	p.SetData(unsafe.Pointer(c))
	log.Info(nil, "[crpc.server] online", log.String("cip", p.GetRealPeerIP()))
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error(nil, "[crpc.server] userdata format wrong", log.String("cip", p.GetRealPeerIP()))
		p.Close(false)
		return
	}
	c := (*client)(p.GetData())
	if msg.Type == MsgType_CANCEL {
		c.RLock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
		}
		c.RUnlock()
		return
	}
	handler, ok := s.handler[msg.Path]
	if !ok {
		log.Error(nil, "[crpc.server] path doesn't exist", log.String("cip", p.GetRealPeerIP()), log.String("path", msg.Path))
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrNoapi
		msg.Metadata = map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			if e == stream.ErrMsgLarge {
				e = cerror.ErrRespmsgLen
			} else if e == stream.ErrConnClosed {
				e = cerror.ErrClosed
			}
			log.Error(nil, "[crpc.server] write response failed", log.String("cip", p.GetRealPeerIP()), log.String("path", msg.Path), log.CError(e))
		}
		return
	}
	c.Lock()
	if c.maxcallid == 0 {
		c.Unlock()
		//tell peer self closed
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrServerClosing
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			if e == stream.ErrMsgLarge {
				e = cerror.ErrRespmsgLen
			} else if e == stream.ErrConnClosed {
				e = cerror.ErrClosed
			}
			log.Error(nil, "[crpc.server] write response failed", log.String("cip", p.GetRealPeerIP()), log.String("path", msg.Path), log.CError(e))
		}
		return
	}
	if e := s.stop.Add(1); e != nil {
		c.Unlock()
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		if e == graceful.ErrClosing {
			//tell peer self closed
			msg.Error = cerror.ErrServerClosing
		} else {
			//tell peer self busy
			msg.Error = cerror.ErrBusy
		}
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			if e == stream.ErrMsgLarge {
				e = cerror.ErrRespmsgLen
			} else if e == stream.ErrConnClosed {
				e = cerror.ErrClosed
			}
			log.Error(nil, "[crpc.server] write response failed", log.String("cip", p.GetRealPeerIP()), log.String("path", msg.Path), log.CError(e))
		}
		return
	}
	c.maxcallid = msg.Callid
	ctx, cancel := context.WithCancel(p)
	c.calls[msg.Callid] = cancel
	c.Unlock()
	go func() {
		defer s.stop.DoneOne()
		handler(ctx, p, msg)
		c.Lock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
			delete(c.calls, msg.Callid)
		}
		c.Unlock()
	}()
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	log.Info(nil, "[crpc.server] offline", log.String("cip", p.GetRealPeerIP()))
}
