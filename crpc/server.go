package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	GlobalTimeout  time.Duration     //global timeout for every rpc call
	ConnectTimeout time.Duration     //default 500ms
	HeartPorbe     time.Duration     //default 1s,3 probe missing means disconnect
	MaxMsgLen      uint32            //default 64M,min 64k
	Certs          map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
}

type CrpcServer struct {
	c              *ServerConfig
	selfappname    string
	tlsc           *tls.Config
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

func NewCrpcServer(c *ServerConfig, selfgroup, selfname string) (*CrpcServer, error) {
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CrpcServer{
		c:              c,
		selfappname:    selfappname,
		global:         make([]OutsideHandler, 0, 10),
		ctxpool:        &sync.Pool{},
		handler:        make(map[string]func(context.Context, *stream.Peer, *Msg), 10),
		handlerTimeout: make(map[string]time.Duration),
		stop:           graceful.New(),
	}
	if len(c.Certs) != 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[crpc.server] load cert:" + cert + " key:" + key + " " + e.Error())
			}
			certificates = append(certificates, temp)
		}
		serverinstance.tlsc = &tls.Config{Certificates: certificates}
	}
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout,
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
	s.instance.RangePeers(func(p *stream.Peer) {
		if tmpdata := p.GetData(); tmpdata != nil {
			c := (*client)(tmpdata)
			c.RLock()
			d, _ := proto.Marshal(&Msg{
				Callid: c.maxcallid + 1,
				Error:  cerror.ErrClosing,
			})
			c.maxcallid = 0
			p.SendMessage(nil, d, nil, nil)
			c.RUnlock()
		}
	})
}

// key path,value timeout(if timeout <= 0 means no timeout)
func (this *CrpcServer) UpdateHandlerTimeout(htcs map[string]time.Duration) {
	tmp := make(map[string]time.Duration)
	for path, timeout := range htcs {
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}
		tmp[path] = timeout
	}
	this.handlerTimeout = tmp
}

func (this *CrpcServer) getHandlerTimeout(path string) time.Duration {
	if t, ok := this.handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout
}

// thread unsafe
func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *CrpcServer) RegisterHandler(path string, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(path, handlers...)
}

func (s *CrpcServer) insidehandler(path string, handlers ...OutsideHandler) func(context.Context, *stream.Peer, *Msg) {
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(ctx context.Context, p *stream.Peer, msg *Msg) {
		sourceip := p.GetRealPeerIP()
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		if len(msg.Tracedata) == 0 || msg.Tracedata["TraceID"] == "" {
			ctx = log.InitTrace(ctx, "", s.selfappname, host.Hostip, "CRPC", msg.Path, 0)
		} else {
			sourceapp = msg.Tracedata["SourceApp"]
			sourcemethod = msg.Tracedata["SourceMethod"]
			sourcepath = msg.Tracedata["SourcePath"]
			clientdeep, e := strconv.Atoi(msg.Tracedata["Deep"])
			if e != nil || sourceapp == "" || sourcemethod == "" || sourcepath == "" || clientdeep == 0 {
				log.Error(nil, "[crpc.server] client:", sourceip, "path:", path, "method: CRPC error: tracedata:", msg.Tracedata, "format wrong")
				p.Close()
				return
			}
			ctx = log.InitTrace(ctx, msg.Tracedata["TraceID"], s.selfappname, host.Hostip, "CRPC", msg.Path, clientdeep)
		}
		traceid, _, _, _, _, selfdeep := log.GetTrace(ctx)
		//var globaldl int64
		start := time.Now()
		var min int64
		servertimeout := int64(s.getHandlerTimeout(path))
		if servertimeout > 0 {
			serverdl := time.Now().UnixNano() + servertimeout
			if msg.Deadline != 0 {
				//compare use the small one
				if msg.Deadline < serverdl {
					min = msg.Deadline
				} else {
					min = serverdl
				}
			} else {
				//use server timeout
				min = serverdl
			}
		} else if msg.Deadline != 0 {
			//use client timeout
			min = msg.Deadline
		} else {
			//no timeout
			min = 0
		}
		if min != 0 {
			if min < start.UnixNano()+int64(time.Millisecond) {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrDeadlineExceeded
				msg.Metadata = nil
				msg.Tracedata = nil
				end := time.Now()
				log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfappname, host.Hostip+":"+p.GetLocalPort(), "CRPC", path, &start, &end, msg.Error)
				monitor.CrpcServerMonitor(sourceapp, "CRPC", path, msg.Error, uint64(end.UnixNano()-start.UnixNano()))
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//logic
		workctx := s.getContext(ctx, p, msg, totalhandlers)
		if _, ok := workctx.metadata["Client-IP"]; !ok {
			workctx.metadata["Client-IP"] = sourceip
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[crpc.server] client:", sourceapp+":"+sourceip, "path:", path, "method: CRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrPanic
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(workctx, d, nil, nil); e != nil {
				if e == stream.ErrMsgLarge {
					msg.Path = ""
					msg.Deadline = 0
					msg.Body = nil
					msg.Error = cerror.ErrRespmsgLen
					msg.Metadata = nil
					msg.Tracedata = nil
					d, _ = proto.Marshal(msg)
					if e = p.SendMessage(workctx, d, nil, nil); e != nil {
						if e == stream.ErrConnClosed {
							msg.Error = cerror.ErrClosed
						} else {
							msg.Error = cerror.ConvertStdError(e)
						}
					}
				} else if e == stream.ErrConnClosed {
					msg.Error = cerror.ErrClosed
				} else {
					msg.Error = cerror.ConvertStdError(e)
				}
			}
			end := time.Now()
			log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfappname, host.Hostip+":"+p.GetLocalPort(), "CRPC", path, &start, &end, msg.Error)
			monitor.CrpcServerMonitor(sourceapp, "CRPC", path, msg.Error, uint64(end.UnixNano()-start.UnixNano()))
			s.putContext(workctx)
		}()
		workctx.run()
	}
}

// return false will close the connection
func (s *CrpcServer) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	if s.stop.Closing() {
		//self closing
		return nil, false
	}
	if common.Byte2str(peerVerifyData) != s.selfappname {
		return nil, false
	}
	return nil, true
}

// return false will close the connection
func (s *CrpcServer) onlinefunc(p *stream.Peer) bool {
	if s.stop.Closing() {
		//tel all peers self closed
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  cerror.ErrClosing,
		})
		p.SendMessage(nil, d, nil, nil)
	}
	c := &client{
		maxcallid: 1,
		calls:     make(map[uint64]context.CancelFunc),
	}
	p.SetData(unsafe.Pointer(c))
	log.Info(nil, "[crpc.server.onlinefunc] client:", p.GetRealPeerIP(), "online")
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error(nil, "[crpc.server.userfunc] client:", p.GetRealPeerIP(), "data format wrong:", e)
		p.Close()
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
		log.Error(nil, "[crpc.server.userfunc] client:", p.GetRealPeerIP(), "call path:", msg.Path, "doesn't exist")
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrNoapi
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			log.Error(nil, "[crpc.server.userfunc] send message to client:", p.GetRealPeerIP(), e)
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
		msg.Error = cerror.ErrClosing
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			log.Error(nil, "[crpc.server.userfunc] send messge to client:", p.GetRealPeerIP(), e)
		}
		return
	}
	if !s.stop.AddOne() {
		c.Unlock()
		//tell peer self closed
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrClosing
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			log.Error(nil, "[crpc.server.userfunc] send message to client:", p.GetRealPeerIP(), e)
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
	log.Info(nil, "[crpc.server.offlinefunc] client:", p.GetRealPeerIP(), "offline")
}
