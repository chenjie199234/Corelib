package crpc

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx *Context)

type ServerConfig struct {
	//when server close,server will wait this time before close,every request will refresh the time
	//min is 1 second
	WaitCloseTime time.Duration
	//global timeout for every rpc call
	GlobalTimeout time.Duration
	HeartPorbe    time.Duration
	GroupNum      uint32
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	CertKeys      map[string]string //mapkey: cert path,mapvalue: key path
	VerifyDatas   []string
}

func (c *ServerConfig) validate() {
	if c.WaitCloseTime < time.Second {
		c.WaitCloseTime = time.Second
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe <= 0 {
		c.HeartPorbe = 1500 * time.Millisecond
	}
	if c.GroupNum == 0 {
		c.GroupNum = 1
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
	c                  *ServerConfig
	tlsc               *tls.Config
	global             []OutsideHandler
	ctxpool            *sync.Pool
	handler            map[string]func(context.Context, string, *Msg)
	instance           *stream.Instance
	closewait          *sync.WaitGroup
	totalreqnum        int32
	refreshclosewaitch chan struct{}
}
type client struct {
	sync.RWMutex
	calls map[uint64]context.CancelFunc
}

func NewCrpcServer(c *ServerConfig, selfgroup, selfname string) (*CrpcServer, error) {
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return nil, e
	}
	appname := selfgroup + "." + selfname
	if e := common.NameCheck(appname, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CrpcServer{
		c:                  c,
		global:             make([]OutsideHandler, 0, 10),
		ctxpool:            &sync.Pool{},
		handler:            make(map[string]func(context.Context, string, *Msg), 10),
		closewait:          &sync.WaitGroup{},
		refreshclosewaitch: make(chan struct{}, 1),
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
		GroupNum:           c.GroupNum,
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
var ErrAlreadyStarted = errors.New("[crpc.server] already started")

func (s *CrpcServer) StartCrpcServer(listenaddr string) error {
	e := s.instance.StartTcpServer(listenaddr, s.tlsc)
	if e == stream.ErrServerClosed {
		return ErrServerClosed
	} else if e == stream.ErrAlreadyStarted {
		return ErrAlreadyStarted
	}
	return e
}
func (s *CrpcServer) StopCrpcServer() {
	defer func() {
		s.closewait.Wait()
	}()
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
			Error:  ERRCLOSING,
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
			case <-s.refreshclosewaitch:
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
func (s *CrpcServer) getContext(ctx context.Context, peeruniquename string, msg *Msg, handlers []OutsideHandler) *Context {
	result, ok := s.ctxpool.Get().(*Context)
	if !ok {
		return &Context{
			Context:        ctx,
			peeruniquename: peeruniquename,
			msg:            msg,
			handlers:       handlers,
		}
	}
	result.Context = ctx
	result.peeruniquename = peeruniquename
	result.msg = msg
	result.handlers = handlers
	return result
}

func (s *CrpcServer) putContext(ctx *Context) {
	ctx.Context = nil
	ctx.peeruniquename = ""
	ctx.msg = nil
	ctx.handlers = nil
	ctx.next = 0
}

//thread unsafe
func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
func (s *CrpcServer) RegisterHandler(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := s.insidehandler(path, functimeout, handlers...)
	if e != nil {
		return e
	}
	s.handler[path] = h
	return nil
}

func (s *CrpcServer) insidehandler(path string, functimeout time.Duration, handlers ...OutsideHandler) (func(context.Context, string, *Msg), error) {
	totalhandlers := make([]OutsideHandler, 1)
	totalhandlers = append(totalhandlers, s.global...)
	totalhandlers = append(totalhandlers, handlers...)
	if len(totalhandlers) > math.MaxInt8 {
		return nil, errors.New("[crpc.server] too many handlers for one path")
	}
	return func(ctx context.Context, peeruniquename string, msg *Msg) {
		var globaldl int64
		var funcdl int64
		now := time.Now()
		if s.c.GlobalTimeout != 0 {
			globaldl = now.UnixNano() + int64(s.c.GlobalTimeout)
		}
		if functimeout != 0 {
			funcdl = now.UnixNano() + int64(functimeout)
		}
		min := int64(math.MaxInt64)
		if msg.Deadline != 0 && msg.Deadline < min {
			min = msg.Deadline
		}
		if funcdl != 0 && funcdl < min {
			min = funcdl
		}
		if globaldl != 0 && globaldl < min {
			min = globaldl
		}
		if min != math.MaxInt64 {
			if min < now.UnixNano()+int64(time.Millisecond) {
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
		if msg.Metadata != nil {
			ctx = metadata.SetAllMetadata(ctx, msg.Metadata)
		}
		//logic
		workctx := s.getContext(ctx, peeruniquename, msg, totalhandlers)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 8192)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[crpc.server] client:", peeruniquename, "path:", path, "panic:", e, "\n"+common.Byte2str(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRPANIC
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			s.putContext(workctx)
		}()
		workctx.Next()
	}, nil
}
func (s *CrpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//self closed
		return nil, false
	}
	temp := common.Byte2str(peerVerifyData)
	index := strings.LastIndex(temp, "|")
	if index == -1 {
		return nil, false
	}
	targetname := temp[index+1:]
	vdata := temp[:index]
	if targetname != s.instance.GetSelfName() {
		return nil, false
	}
	if len(s.c.VerifyDatas) == 0 {
		dup := make([]byte, len(vdata))
		copy(dup, vdata)
		return dup, true
	}
	for _, verifydata := range s.c.VerifyDatas {
		if vdata == verifydata {
			return common.Str2byte(verifydata), true
		}
	}
	return nil, false
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
	var traceid string
	if msg.Tracedata != nil {
		traceid = msg.Tracedata["Traceid"]
	}
	tracectx := trace.InitTrace(p, traceid, s.instance.GetSelfName(), host.Hostip, "CRPC", msg.Path)
	//if traceid is not empty,traceid will not change
	//if traceid is empty,init trace will create a new traceid,use the new traceid
	traceid, _, _, _, _ = trace.GetTrace(tracectx)
	path := msg.Path
	handler, ok := s.handler[msg.Path]
	if !ok {
		log.Error(tracectx, "[crpc.server.userfunc] client:", p.GetPeerUniqueName(), "call path:", msg.Path, "error: unknown path")
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = ERRNOAPI
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
			case s.refreshclosewaitch <- struct{}{}:
			default:
			}
			//tell peer self closed
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = ERRCLOSING
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
		//logic
		start := time.Now()
		handler(ctx, p.GetPeerUniqueName(), msg)
		end := time.Now()
		trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath), trace.SERVER, s.instance.GetSelfName(), host.Hostip, "CRPC", path, &start, &end, msg.Error)
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(ctx, d, nil, nil); e != nil {
			log.Error(ctx, "[crpc.server.userfunc] send message to client:", p.GetPeerUniqueName(), "error:", e)
			if e == stream.ErrMsgLarge {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRRESPMSGLARGE
				msg.Metadata = nil
				msg.Tracedata = nil
				d, _ = proto.Marshal(msg)
				p.SendMessage(ctx, d, nil, nil)
			}
		}
		if atomic.LoadInt32(&s.totalreqnum) < 0 {
			select {
			case s.refreshclosewaitch <- struct{}{}:
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
