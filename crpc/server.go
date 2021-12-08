package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"math"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	//when server close,server will wait this time before close,every request will refresh the time
	//min is 1 second
	WaitCloseTime time.Duration
	GlobalTimeout time.Duration //global timeout for every rpc call(including connection establish time)
	HeartPorbe    time.Duration //default 1s,3 probe missing means disconnect
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	CertKeys      map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.WaitCloseTime < time.Second {
		c.WaitCloseTime = time.Second
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe < time.Second {
		c.HeartPorbe = time.Second
	}
	if c.SocketRBuf == 0 {
		c.SocketRBuf = 1024
	}
	if c.SocketRBuf > 65535 {
		c.SocketRBuf = 65535
	}
	if c.SocketWBuf == 0 {
		c.SocketWBuf = 1024
	}
	if c.SocketWBuf > 65535 {
		c.SocketWBuf = 65535
	}
	if c.MaxMsgLen < 1024 {
		c.MaxMsgLen = 65535
	}
	if c.MaxMsgLen > 65535 {
		c.MaxMsgLen = 65535
	}
}

type CrpcServer struct {
	c                *ServerConfig
	tlsc             *tls.Config
	global           []OutsideHandler
	ctxpool          *sync.Pool
	handler          map[string]func(context.Context, *stream.Peer, *Msg)
	handlerTimeout   map[string]time.Duration
	instance         *stream.Instance
	closewait        *sync.WaitGroup
	totalreqnum      int32
	refreshclosewait chan *struct{}
}
type client struct {
	sync.RWMutex
	calls map[uint64]context.CancelFunc
}

func NewCrpcServer(c *ServerConfig, selfgroup, selfname string) (*CrpcServer, error) {
	appname := selfgroup + "." + selfname
	if e := name.FullCheck(appname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CrpcServer{
		c:                c,
		global:           make([]OutsideHandler, 0, 10),
		ctxpool:          &sync.Pool{},
		handler:          make(map[string]func(context.Context, *stream.Peer, *Msg), 10),
		handlerTimeout:   make(map[string]time.Duration),
		closewait:        &sync.WaitGroup{},
		refreshclosewait: make(chan *struct{}, 1),
	}
	if len(c.CertKeys) != 0 {
		certificates := make([]tls.Certificate, 0, len(c.CertKeys))
		for cert, key := range c.CertKeys {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[crpc.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		serverinstance.tlsc = &tls.Config{Certificates: certificates}
	}
	serverinstance.closewait.Add(1)
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
		TcpC: &stream.TcpConfig{
			SocketRBufLen: c.SocketRBuf,
			SocketWBufLen: c.SocketWBuf,
			MaxMsgLen:     c.MaxMsgLen,
		},
	}
	instancec.Verifyfunc = serverinstance.verifyfunc
	instancec.Onlinefunc = serverinstance.onlinefunc
	instancec.Userdatafunc = serverinstance.userfunc
	instancec.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance, _ = stream.NewInstance(instancec, selfgroup, selfname)
	return serverinstance, nil
}

var ErrServerClosed = errors.New("[crpc.server] closed")

func (s *CrpcServer) StartCrpcServer(listenaddr string) error {
	e := s.instance.StartTcpServer(listenaddr, s.tlsc)
	if e == stream.ErrServerClosed {
		return ErrServerClosed
	}
	return e
}
func (s *CrpcServer) StopCrpcServer() {
	defer s.closewait.Wait()
	stop := false
	for {
		old := atomic.LoadInt32(&s.totalreqnum)
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old-math.MaxInt32) {
				stop = true
				break
			}
		} else {
			break
		}
	}
	if stop {
		//tel all peers self closed
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  errClosing,
		})
		s.instance.SendMessageAll(nil, d, nil, nil)
		//wait at least s.c.WaitCloseTime before stop the under layer socket
		tmer := time.NewTimer(s.c.WaitCloseTime)
		for {
			select {
			case <-tmer.C:
				if atomic.LoadInt32(&s.totalreqnum) != -math.MaxInt32 {
					tmer.Reset(s.c.WaitCloseTime)
				} else {
					s.instance.Stop()
					s.closewait.Done()
					return
				}
			case <-s.refreshclosewait:
				tmer.Reset(s.c.WaitCloseTime)
				for len(tmer.C) > 0 {
					<-tmer.C
				}
			}
		}
	}
}
func (s *CrpcServer) GetClientNum() int32 {
	return s.instance.GetPeerNum()
}
func (s *CrpcServer) GetReqNum() int32 {
	totalreqnum := atomic.LoadInt32(&s.totalreqnum)
	if totalreqnum < 0 {
		return totalreqnum + math.MaxInt32
	} else {
		return totalreqnum
	}
}

func (this *CrpcServer) UpdateHandlerTimeout(htcs map[string]time.Duration) {
	tmp := make(map[string]time.Duration)
	for path, timeout := range htcs {
		if timeout == 0 {
			//jump,0 means no handler specific timeout
			continue
		}
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}
		tmp[path] = timeout
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout)), unsafe.Pointer(&tmp))
}

func (this *CrpcServer) getHandlerTimeout(path string) time.Duration {
	handlerTimeout := *(*map[string]time.Duration)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout))))
	if t, ok := handlerTimeout[path]; ok {
		if this.c.GlobalTimeout <= t {
			return this.c.GlobalTimeout
		}
		return t
	}
	return this.c.GlobalTimeout
}

//thread unsafe
func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
func (s *CrpcServer) RegisterHandler(path string, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(path, handlers...)
}

func (s *CrpcServer) insidehandler(path string, handlers ...OutsideHandler) func(context.Context, *stream.Peer, *Msg) {
	totalhandlers := append(s.global, handlers...)
	return func(ctx context.Context, p *stream.Peer, msg *Msg) {
		traceid, _, _, _, _, selfdeep := trace.GetTrace(ctx)
		var sourceapp, sourceip, sourcemethod, sourcepath string
		if msg.Tracedata != nil {
			sourcemethod = msg.Tracedata["SourceMethod"]
			sourcepath = msg.Tracedata["SourcePath"]
		}
		sourceapp = p.GetPeerName()
		if sourceapp == "" {
			sourceapp = "unkown"
		}
		sourceip = p.GetRemoteAddr()
		if sourcemethod == "" {
			sourcemethod = "unknown"
		}
		if sourcepath == "" {
			sourcepath = "unknown"
		}
		//var globaldl int64
		start := time.Now()
		servertimeout := int64(s.getHandlerTimeout(path))
		var min int64
		if msg.Deadline != 0 && servertimeout != 0 {
			serverdl := start.UnixNano() + servertimeout
			if msg.Deadline <= serverdl {
				min = msg.Deadline
			} else {
				min = serverdl
			}
		} else if msg.Deadline != 0 {
			min = msg.Deadline
		} else if s.c.GlobalTimeout != 0 {
			min = start.UnixNano() + servertimeout
		}
		if min != 0 {
			if min < start.UnixNano()+int64(time.Millisecond) {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrDeadlineExceeded
				msg.Metadata = nil
				msg.Tracedata = nil
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//logic
		workctx := s.getContext(ctx, p, msg, totalhandlers)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[crpc.server] client:", p.GetPeerUniqueName(), "path:", path, "method: CRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ErrPanic
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			end := time.Now()
			trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), trace.SERVER, s.instance.GetSelfName(), host.Hostip, "CRPC", path, &start, &end, msg.Error)
			s.putContext(workctx)
		}()
		workctx.run()
	}
}
func (s *CrpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//self closed
		return nil, false
	}
	if common.Byte2str(peerVerifyData) != s.instance.GetSelfName() {
		return nil, false
	}
	return nil, true
}
func (s *CrpcServer) onlinefunc(p *stream.Peer) bool {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//self closed
		return false
	}
	c := &client{
		calls: make(map[uint64]context.CancelFunc),
	}
	p.SetData(unsafe.Pointer(c))
	log.Info(nil, "[crpc.server.onlinefunc] client:", p.GetPeerUniqueName(), "online")
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error(nil, "[crpc.server.userfunc] client:", p.GetPeerUniqueName(), "data format error:", e)
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
	var tracectx context.Context
	if len(msg.Tracedata) == 0 || msg.Tracedata["Traceid"] == "" {
		tracectx = trace.InitTrace(p, "", s.instance.GetSelfName(), host.Hostip, "CRPC", msg.Path, 0)
	} else if len(msg.Tracedata) != 4 || msg.Tracedata["Deep"] == "" {
		log.Error(nil, "[crpc.server.userfunc] client:", p.GetPeerUniqueName(), "path:", msg.Path, "method: CRPC error: tracedata:", msg.Tracedata, "format error")
		p.Close()
		return
	} else if clientdeep, e := strconv.Atoi(msg.Tracedata["Deep"]); e != nil {
		log.Error(nil, "[crpc.server.userfunc] client:", p.GetPeerUniqueName(), "path:", msg.Path, "method: CRPC error: tracedata:", msg.Tracedata, "format error")
		p.Close()
		return
	} else {
		tracectx = trace.InitTrace(p, msg.Tracedata["Traceid"], s.instance.GetSelfName(), host.Hostip, "CRPC", msg.Path, clientdeep)
	}
	handler, ok := s.handler[msg.Path]
	if !ok {
		log.Error(tracectx, "[crpc.server.userfunc] client:", p.GetPeerUniqueName(), "call path:", msg.Path, "error: unknown path")
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = ErrNoapi
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(tracectx, d, nil, nil); e != nil {
			log.Error(tracectx, "[crpc.server.userfunc] send message to client:", p.GetPeerUniqueName(), "error:", e)
		}
		return
	}
	for {
		old := atomic.LoadInt32(&s.totalreqnum)
		if old >= 0 {
			//add req num
			if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old+1) {
				break
			}
		} else {
			//refresh close wait
			select {
			case s.refreshclosewait <- nil:
			default:
			}
			//tell peer self closed
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = errClosing
			msg.Metadata = nil
			msg.Tracedata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(tracectx, d, nil, nil); e != nil {
				log.Error(tracectx, "[crpc.server.userfunc] send message to client:", p.GetPeerUniqueName(), "error:", e)
			}
			return
		}
	}
	ctx, cancel := context.WithCancel(tracectx)
	c.Lock()
	c.calls[msg.Callid] = cancel
	c.Unlock()
	go func() {
		handler(ctx, p, msg)
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(ctx, d, nil, nil); e != nil {
			log.Error(ctx, "[crpc.server.userfunc] send message to client:", p.GetPeerUniqueName(), "error:", e)
			if e == stream.ErrMsgLarge {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ErrRespmsgLen
				msg.Metadata = nil
				msg.Tracedata = nil
				d, _ = proto.Marshal(msg)
				p.SendMessage(ctx, d, nil, nil)
			}
		}
		if atomic.LoadInt32(&s.totalreqnum) < 0 {
			select {
			case s.refreshclosewait <- nil:
			default:
			}
		}
		atomic.AddInt32(&s.totalreqnum, -1)
		c.Lock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
			delete(c.calls, msg.Callid)
		}
		c.Unlock()
	}()
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	log.Info(nil, "[crpc.server.offlinefunc] client:", p.GetPeerUniqueName(), "offline")
}
