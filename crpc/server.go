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
	GlobalTimeout time.Duration //global timeout for every rpc call(including connection establish time)
	HeartPorbe    time.Duration //default 1s,3 probe missing means disconnect
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	CertKeys      map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
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
	c              *ServerConfig
	tlsc           *tls.Config
	global         []OutsideHandler
	ctxpool        *sync.Pool
	handler        map[string]func(context.Context, *stream.Peer, *Msg)
	handlerTimeout map[string]time.Duration
	instance       *stream.Instance
	closewait      *sync.WaitGroup
	closewaittimer *time.Timer
	totalreqnum    int32
}
type client struct {
	sync.RWMutex
	maxcallid uint64 //0-server is closing
	calls     map[uint64]context.CancelFunc
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
		c:              c,
		global:         make([]OutsideHandler, 0, 10),
		ctxpool:        &sync.Pool{},
		handler:        make(map[string]func(context.Context, *stream.Peer, *Msg), 10),
		handlerTimeout: make(map[string]time.Duration),
		closewait:      &sync.WaitGroup{},
		closewaittimer: time.NewTimer(0),
	}
	<-serverinstance.closewaittimer.C
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
	defer func() {
		s.closewait.Wait()
		s.instance.Stop()
	}()
	s.instance.PreStop()
	var old int32
	for {
		old = atomic.LoadInt32(&s.totalreqnum)
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old-math.MaxInt32) {
				break
			}
		} else {
			return
		}
	}
	//tel all peers self closed
	s.instance.RangePeers(func(p *stream.Peer) {
		if tmpdata := p.GetData(); tmpdata != nil {
			c := (*client)(tmpdata)
			c.RLock()
			d, _ := proto.Marshal(&Msg{
				Callid: c.maxcallid + 1,
				Error:  errClosing,
			})
			c.maxcallid = 0
			p.SendMessage(nil, d, nil, nil)
			c.RUnlock()
		}
	})
	//if there are calls,done status will be checked when each call finishes
	//this is used to prevent there are no calls
	if old == 0 {
		s.closewait.Done()
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
		if timeout <= 0 {
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
	if t, ok := handlerTimeout[path]; ok && (this.c.GlobalTimeout == 0 || t < this.c.GlobalTimeout) {
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

//return false will close the connection
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

//return false will close the connection
func (s *CrpcServer) onlinefunc(p *stream.Peer) bool {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//tel all peers self closed
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  errClosing,
		})
		p.SendMessage(nil, d, nil, nil)
	}
	c := &client{
		maxcallid: 1,
		calls:     make(map[uint64]context.CancelFunc),
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
		if old < 0 {
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
		//add req num
		if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old+1) {
			break
		}
	}
	c.Lock()
	if c.maxcallid == 0 {
		c.Unlock()
		//tell peer self closed
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = errClosing
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(tracectx, d, nil, nil); e != nil {
			log.Error(tracectx, "[crpc.server.userfunc] send messge to client:", p.GetPeerUniqueName(), "error:", e)
		}
		if atomic.AddInt32(&s.totalreqnum, -1) == -math.MaxInt32 {
			s.closewait.Done()
		}
		return
	}
	c.maxcallid = msg.Callid
	ctx, cancel := context.WithCancel(tracectx)
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
		c.Lock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
			delete(c.calls, msg.Callid)
		}
		c.Unlock()
		if atomic.AddInt32(&s.totalreqnum, -1) == -math.MaxInt32 {
			s.closewait.Done()
		}
	}()
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	log.Info(nil, "[crpc.server.offlinefunc] client:", p.GetPeerUniqueName(), "offline")
}
