package rpc

import (
	"context"
	"errors"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	GlobalTimeout          time.Duration
	HeartTimeout           time.Duration
	HeartPorbe             time.Duration
	GroupNum               uint32
	SocketRBuf             uint32
	SocketWBuf             uint32
	MaxMsgLen              uint32
	MaxBufferedWriteMsgNum uint32
	VerifyDatas            []string
}

func (c *ServerConfig) validate() {
	if c.WaitCloseTime < time.Second {
		c.WaitCloseTime = time.Second
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartTimeout <= 0 {
		c.HeartTimeout = 5 * time.Second
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
	if c.MaxBufferedWriteMsgNum == 0 {
		c.MaxBufferedWriteMsgNum = 256
	}
}

type RpcServer struct {
	c                  *ServerConfig
	global             []OutsideHandler
	ctxpool            *sync.Pool
	handler            map[string]func(context.Context, string, *Msg)
	instance           *stream.Instance
	closewait          *sync.WaitGroup
	totalreqnum        int32
	refreshclosewaitch chan struct{}
}

func NewRpcServer(c *ServerConfig, selfgroup, selfname string) (*RpcServer, error) {
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
	serverinstance := &RpcServer{
		c:                  c,
		global:             make([]OutsideHandler, 0, 10),
		ctxpool:            &sync.Pool{},
		handler:            make(map[string]func(context.Context, string, *Msg), 10),
		closewait:          &sync.WaitGroup{},
		refreshclosewaitch: make(chan struct{}, 1),
	}
	serverinstance.closewait.Add(1)
	instancec := &stream.InstanceConfig{
		HeartbeatTimeout:       c.HeartTimeout,
		HeartprobeInterval:     c.HeartPorbe,
		MaxBufferedWriteMsgNum: c.MaxBufferedWriteMsgNum,
		GroupNum:               c.GroupNum,
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

var ErrServerClosed = errors.New("[rpc.server] closed")
var ErrAlreadyStarted = errors.New("[rpc.server] already started")

func (s *RpcServer) StartRpcServer(listenaddr string) error {
	e := s.instance.StartTcpServer(listenaddr, nil)
	if e == stream.ErrServerClosed {
		return ErrServerClosed
	} else if e == stream.ErrAlreadyStarted {
		return ErrAlreadyStarted
	}
	return e
}
func (s *RpcServer) StopRpcServer() {
	defer func() {
		s.closewait.Wait()
	}()
	stop := false
	for {
		old := s.totalreqnum
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
			Error:  ERRCLOSING.Error(),
		})
		s.instance.SendMessageAll(d, true)
		//wait at least s.c.WaitCloseTime before stop the under layer socket
		tmer := time.NewTimer(s.c.WaitCloseTime)
		for {
			select {
			case <-tmer.C:
				if s.totalreqnum != -math.MaxInt32 {
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
func (s *RpcServer) GetClientNum() int32 {
	return s.instance.GetPeerNum()
}
func (s *RpcServer) GetReqNum() int32 {
	totalreqnum := atomic.LoadInt32(&s.totalreqnum)
	if totalreqnum < 0 {
		return totalreqnum + math.MaxInt32
	} else {
		return totalreqnum
	}
}
func (s *RpcServer) getContext(ctx context.Context, peeruniquename string, msg *Msg, handlers []OutsideHandler) *Context {
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

func (s *RpcServer) putContext(ctx *Context) {
	ctx.Context = nil
	ctx.peeruniquename = ""
	ctx.msg = nil
	ctx.handlers = nil
	ctx.next = 0
}

//thread unsafe
func (s *RpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
func (s *RpcServer) RegisterHandler(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := s.insidehandler(path, functimeout, handlers...)
	if e != nil {
		return e
	}
	s.handler[path] = h
	return nil
}

func (s *RpcServer) insidehandler(path string, functimeout time.Duration, handlers ...OutsideHandler) (func(context.Context, string, *Msg), error) {
	totalhandlers := make([]OutsideHandler, 1)
	totalhandlers = append(totalhandlers, s.global...)
	totalhandlers = append(totalhandlers, handlers...)
	if len(totalhandlers) > math.MaxInt8 {
		return nil, errors.New("[rpc.server] too many handlers for one path")
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
				msg.Error = context.DeadlineExceeded.Error()
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
				log.Error(workctx, "[rpc.server] client:", peeruniquename, "path:", path, "panic:", e, "\n"+common.Byte2str(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRPANIC.Error()
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			s.putContext(workctx)
		}()
		workctx.Next()
	}, nil
}
func (s *RpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if s.totalreqnum < 0 {
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
func (s *RpcServer) onlinefunc(p *stream.Peer, peeruniquename string, sid int64) bool {
	if s.totalreqnum < 0 {
		//self closed
		return false
	}
	log.Info(nil, "[rpc.server.onlinefunc] client:", peeruniquename, "online")
	return true
}
func (s *RpcServer) userfunc(p *stream.Peer, peeruniquename string, data []byte, sid int64) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error(nil, "[rpc.server.userfunc] client:", peeruniquename, "data format error:", e)
		p.Close(sid)
		return
	}
	var traceid string
	if msg.Tracedata != nil {
		traceid = msg.Tracedata["Traceid"]
	}
	ctx := trace.InitTrace(nil, traceid, s.instance.GetSelfName(), host.Hostip, "rpc", msg.Path, trace.RPC)
	//if traceid is not empty,traceid will not change
	//if traceid is empty,init trace will create a new traceid,use the new traceid
	traceid, _, _, _, _, _ = trace.GetTrace(ctx)
	handler, ok := s.handler[msg.Path]
	if !ok {
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = ERRNOAPI.Error()
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(d, sid, true); e != nil {
			log.Error(ctx, "[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
		}
		return
	}
	for {
		old := s.totalreqnum
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
			msg.Error = ERRCLOSING.Error()
			msg.Metadata = nil
			msg.Tracedata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, sid, true); e != nil {
				log.Error(ctx, "[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
			}
			return
		}
	}
	go func() {
		var fromapp, fromip, frommethod, frompath string
		var fromkind trace.KIND
		if msg.Tracedata != nil {
			frommethod = msg.Tracedata["Method"]
			frompath = msg.Tracedata["Path"]
			fromkind = trace.KIND(msg.Tracedata["Kind"])
		}
		fromapp = peeruniquename[:strings.Index(peeruniquename, ":")]
		if fromapp == "" {
			fromapp = "unkown"
		}
		fromip = peeruniquename[strings.Index(peeruniquename, ":")+1 : strings.LastIndex(peeruniquename, ":")]
		if frommethod == "" {
			frommethod = "unknown"
		}
		if frompath == "" {
			frompath = "unknown"
		}
		if fromkind == "" {
			fromkind = trace.KIND("unknown")
		}
		traceend := trace.TraceStart(trace.InitTrace(nil, traceid, fromapp, fromip, frommethod, frompath, fromkind), trace.SERVER, s.instance.GetSelfName(), host.Hostip, "RPC", msg.Path, trace.RPC)
		//logic
		handler(ctx, peeruniquename, msg)
		traceend(cerror.ErrorstrToError(msg.Error))
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(d, sid, true); e != nil {
			log.Error(ctx, "[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
			if e == stream.ErrMsgLarge {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRRESPMSGLARGE.Error()
				msg.Metadata = nil
				msg.Tracedata = nil
				d, _ = proto.Marshal(msg)
				p.SendMessage(d, sid, true)
			}
		}
		if s.totalreqnum < 0 {
			select {
			case s.refreshclosewaitch <- struct{}{}:
			default:
			}
		}
		atomic.AddInt32(&s.totalreqnum, -1)
	}()
}
func (s *RpcServer) offlinefunc(p *stream.Peer, peeruniquename string) {
	log.Info(nil, "[rpc.server.offlinefunc] client:", peeruniquename, "offline")
}
