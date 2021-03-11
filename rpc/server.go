package rpc

import (
	"context"
	"errors"
	"math"
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

type RpcServer struct {
	timeout    time.Duration
	global     []OutsideHandler
	ctxpool    *sync.Pool
	handler    map[string]func(string, *Msg)
	instance   *stream.Instance
	verifydata []byte
	status     int32 //0 stop,1 starting
	stopch     chan struct{}
}

func NewRpcServer(c *stream.InstanceConfig, group, name string, vdata []byte, globaltimeout time.Duration) (*RpcServer, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	appname := group + "." + name
	if e := common.NameCheck(appname, true, true, false, true); e != nil {
		return nil, e
	}
	serverinstance := &RpcServer{
		timeout:    globaltimeout,
		global:     make([]OutsideHandler, 0, 10),
		ctxpool:    &sync.Pool{},
		handler:    make(map[string]func(string, *Msg), 10),
		verifydata: vdata,
		stopch:     make(chan struct{}, 1),
	}
	var dupc stream.InstanceConfig
	if c == nil {
		dupc = stream.InstanceConfig{}
	} else {
		dupc = *c //duplicate to remote the callback func race
	}
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = serverinstance.onlinefunc
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = nil
	serverinstance.instance, _ = stream.NewInstance(&dupc, group, name)
	return serverinstance, nil
}
func (s *RpcServer) StartRpcServer(listenaddr string) error {
	if atomic.SwapInt32(&s.status, 1) == 1 {
		return nil
	}
	return s.instance.StartTcpServer(listenaddr)
}
func (s *RpcServer) StopRpcServer() {
	if atomic.SwapInt32(&s.status, 0) == 0 {
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
			s.instance.Stop()
			return
		case <-s.stopch:
			timer.Reset(time.Second)
			for len(timer.C) > 0 {
				<-timer.C
			}
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
	h, e := s.insidehandler(functimeout, handlers...)
	if e != nil {
		return e
	}
	s.handler[path] = h
	return nil
}
func (s *RpcServer) insidehandler(functimeout time.Duration, handlers ...OutsideHandler) (func(string, *Msg), error) {
	totalhandlers := make([]OutsideHandler, 1)
	totalhandlers = append(totalhandlers, s.global...)
	totalhandlers = append(totalhandlers, handlers...)
	if len(totalhandlers) > math.MaxInt8 {
		return nil, errors.New("[rpc.server] too many handlers for one single path")
	}
	return func(peeruniquename string, msg *Msg) {
		defer func() {
			if e := recover(); e != nil {
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
		if s.timeout != 0 {
			globaldl = now.UnixNano() + int64(s.timeout)
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
		if len(msg.Metadata) > 0 {
			ctx = metadata.SetAllMetadata(ctx, msg.Metadata)
		}
		workctx := s.getContext(ctx, peeruniquename, msg, totalhandlers)
		workctx.Next()
		s.putContext(workctx)
	}, nil
}
func (s *RpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if atomic.LoadInt32(&s.status) == 0 {
		return nil, false
	}
	temp := common.Byte2str(peerVerifyData)
	index := strings.LastIndex(temp, "|")
	if index == -1 {
		return nil, false
	}
	targetname := temp[index+1:]
	vdata := temp[:index]
	if targetname != s.instance.GetSelfName() || vdata != common.Byte2str(s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}
func (s *RpcServer) onlinefunc(p *stream.Peer, peeruniquename string, starttime uint64) {
	if atomic.LoadInt32(&s.status) == 0 {
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  ERRCLOSING.Error(),
		})
		select {
		case s.stopch <- struct{}{}:
		default:
		}
		p.SendMessage(d, starttime, true)
	}
}
func (s *RpcServer) userfunc(p *stream.Peer, peeruniquename string, data []byte, starttime uint64) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error("[rpc.server.userfunc] data format error:", e)
		p.Close()
		return
	}
	go func() {
		if atomic.LoadInt32(&s.status) == 0 {
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
				log.Error("[rpc.server.userfunc] send message error:", e)
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
				log.Error("[rpc.server.userfunc] send message error:", e)
			}
			return
		}
		handler(peeruniquename, msg)
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(d, starttime, true); e != nil {
			log.Error("[rpc.server.userfunc] send message error:", e)
		}
	}()
}
