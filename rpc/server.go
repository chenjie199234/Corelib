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

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/metadata"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx *Context)

type ServerConfig struct {
	GlobalTimeout          time.Duration //global timeout for every rpc call
	HeartTimeout           time.Duration
	HeartPorbe             time.Duration
	GroupNum               uint
	SocketRBuf             uint
	SocketWBuf             uint
	MaxMsgLen              uint
	MaxBufferedWriteMsgNum uint
	VerifyDatas            []string
}

func (c *ServerConfig) validate() {
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
	c        *ServerConfig
	global   []OutsideHandler
	ctxpool  *sync.Pool
	handler  map[string]func(string, *Msg)
	instance *stream.Instance
	status   int32 //0-created,not started 1-started 2-closed
	stopch   chan struct{}

	reqnum int32
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
		c:       c,
		global:  make([]OutsideHandler, 0, 10),
		ctxpool: &sync.Pool{},
		handler: make(map[string]func(string, *Msg), 10),
		stopch:  make(chan struct{}, 1),
	}
	instancec := &stream.InstanceConfig{
		HeartbeatTimeout:   c.HeartTimeout,
		HeartprobeInterval: c.HeartPorbe,
		GroupNum:           c.GroupNum,
		TcpC: &stream.TcpConfig{
			SocketRBufLen:          c.SocketRBuf,
			SocketWBufLen:          c.SocketWBuf,
			MaxMsgLen:              c.MaxMsgLen,
			MaxBufferedWriteMsgNum: c.MaxBufferedWriteMsgNum,
		},
	}
	instancec.Verifyfunc = serverinstance.verifyfunc
	instancec.Onlinefunc = serverinstance.onlinefunc
	instancec.Userdatafunc = serverinstance.userfunc
	instancec.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance, _ = stream.NewInstance(instancec, selfgroup, selfname)
	return serverinstance, nil
}
func (s *RpcServer) StartRpcServer(listenaddr string) error {
	if !atomic.CompareAndSwapInt32(&s.status, 0, 1) {
		return nil
	}
	log.Info("[rpc.server] start with verifydatas:", s.c.VerifyDatas)
	return s.instance.StartTcpServer(listenaddr)
}
func (s *RpcServer) StopRpcServer() {
	if atomic.SwapInt32(&s.status, 2) == 2 {
		return
	}
	d, _ := proto.Marshal(&Msg{
		Callid: 0,
		Error:  ERRCLOSING.Error(),
	})
	s.instance.SendMessageAll(d, true)
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-timer.C:
			if s.reqnum != 0 {
				timer.Reset(time.Second)
			} else {
				s.instance.Stop()
				return
			}
		case <-s.stopch:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(time.Second)
		}
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
func (s *RpcServer) insidehandler(path string, functimeout time.Duration, handlers ...OutsideHandler) (func(string, *Msg), error) {
	totalhandlers := make([]OutsideHandler, 1)
	totalhandlers = append(totalhandlers, s.global...)
	totalhandlers = append(totalhandlers, handlers...)
	if len(totalhandlers) > math.MaxInt8 {
		return nil, errors.New("[rpc.server] too many handlers for one single path")
	}
	return func(peeruniquename string, msg *Msg) {
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 8192)
				n := runtime.Stack(stack, false)
				log.Error("[rpc.server] client:", peeruniquename, "path:", path, "panic:", e, "\n"+common.Byte2str(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRPANIC.Error()
				msg.Metadata = nil
			}
		}()
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
		ctx := context.Background()
		if min != math.MaxInt64 {
			if min < now.UnixNano()+int64(time.Millisecond) {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = ERRCTXTIMEOUT.Error()
				msg.Metadata = nil
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//delete port info
		if msg.Metadata != nil {
			ctx = metadata.SetAllMetadata(ctx, msg.Metadata)
		}
		workctx := s.getContext(ctx, peeruniquename, msg, totalhandlers)
		workctx.Next()
		s.putContext(workctx)
	}, nil
}
func (s *RpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if atomic.LoadInt32(&s.status) != 1 {
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
func (s *RpcServer) onlinefunc(p *stream.Peer, peeruniquename string, starttime uint64) {
	if atomic.LoadInt32(&s.status) != 1 {
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  ERRCLOSING.Error(),
		})
		select {
		case s.stopch <- struct{}{}:
		default:
		}
		p.SendMessage(d, starttime, true)
	} else {
		log.Info("[rpc.server.onlinefunc] client:", peeruniquename, "online")
	}
}
func (s *RpcServer) userfunc(p *stream.Peer, peeruniquename string, data []byte, starttime uint64) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error("[rpc.server.userfunc] client:", peeruniquename, "data format error:", e)
		p.Close()
		return
	}
	go func() {
		if atomic.LoadInt32(&s.status) != 1 {
			select {
			case s.stopch <- struct{}{}:
			default:
			}
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = ERRCLOSING.Error()
			msg.Metadata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, starttime, true); e != nil {
				log.Error("[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
			}
			return
		}
		handler, ok := s.handler[msg.Path]
		if !ok {
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = ERRNOAPI.Error()
			msg.Metadata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, starttime, true); e != nil {
				log.Error("[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
			}
			return
		}
		atomic.AddInt32(&s.reqnum, 1)
		defer atomic.AddInt32(&s.reqnum, -1)
		handler(peeruniquename, msg)
		d, _ := proto.Marshal(msg)
		if len(d) > int(s.c.MaxMsgLen) {
			log.Error("[rpc.server.userfunc] send message to client:", peeruniquename, "error:message too large")
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = ERRRESPMSGLARGE.Error()
			msg.Metadata = nil
			d, _ = proto.Marshal(msg)
		}
		if e := p.SendMessage(d, starttime, true); e != nil {
			log.Error("[rpc.server.userfunc] send message to client:", peeruniquename, "error:", e)
		}
		//double check server status
		if atomic.LoadInt32(&s.status) != 1 {
			select {
			case s.stopch <- struct{}{}:
			default:
			}
		}
	}()
}
func (s *RpcServer) offlinefunc(p *stream.Peer, peeruniquename string, starttime uint64) {
	log.Info("[rpc.server.offlinefunc] client:", peeruniquename, "offline")
}
