package rpc

import (
	"context"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/cpu"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e error)

type RpcServer struct {
	c          *stream.InstanceConfig
	timeout    time.Duration
	handler    map[string]func(string, *Msg)
	instance   *stream.Instance
	verifydata []byte
	status     int32 //0 stop,1 starting
	count      int32
	stopch     chan struct{}
}

func NewRpcServer(c *stream.InstanceConfig, globaltimeout time.Duration, vdata []byte) *RpcServer {
	serverinstance := &RpcServer{
		timeout:    globaltimeout,
		handler:    make(map[string]func(string, *Msg), 10),
		verifydata: vdata,
		stopch:     make(chan struct{}, 1),
	}
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = serverinstance.onlinefunc
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = nil
	serverinstance.c = &dupc
	serverinstance.instance = stream.NewInstance(&dupc)
	return serverinstance
}
func (s *RpcServer) StartRpcServer(listenaddr string) {
	if atomic.SwapInt32(&s.status, 1) == 1 {
		return
	}
	s.instance.StartTcpServer(listenaddr)
}
func (s *RpcServer) StopMrpcServer() {
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
			if atomic.LoadInt32(&s.count) == 0 {
				s.instance.Stop()
				return
			}
			timer.Reset(time.Second)
			for len(timer.C) > 0 {
				<-timer.C
			}
		case <-s.stopch:
			timer.Reset(time.Second)
			for len(timer.C) > 0 {
				<-timer.C
			}
		}
	}
}
func (s *RpcServer) insidehandler(functimeout time.Duration, handlers ...OutsideHandler) func(string, *Msg) {
	return func(peeruniquename string, msg *Msg) {
		defer func() {
			if e := recover(); e != nil {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
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
				msg.Cpu = cpu.GetUse()
				msg.Error = ERRCTXTIMEOUT.Error()
				msg.Metadata = nil
				return
			}
			ctx = SetDeadline(ctx, min)
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		ctx = SetPath(ctx, msg.Path)
		ctx = SetPeerName(ctx, peeruniquename)
		if len(msg.Metadata) > 0 {
			ctx = SetAllMetadata(ctx, msg.Metadata)
		}
		var resp []byte
		var err error
		for _, handler := range handlers {
			resp, err = handler(ctx, msg.Body)
			if err != nil {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
				msg.Error = err.Error()
				msg.Metadata = nil
				return
			}
		}
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = resp
		msg.Cpu = cpu.GetUse()
		msg.Error = ""
		msg.Metadata = nil
	}
}

//thread unsafe
func (s *RpcServer) RegisterHandler(path string, functimeout time.Duration, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(functimeout, handlers...)
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
	if targetname != s.c.SelfName || vdata != common.Byte2str(s.verifydata) {
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
		atomic.AddInt32(&s.count, 1)
		defer atomic.AddInt32(&s.count, -1)
		if atomic.LoadInt32(&s.status) == 0 {
			select {
			case s.stopch <- struct{}{}:
			default:
			}
			msg.Path = ""
			msg.Deadline = 0
			msg.Cpu = 0
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
			msg.Cpu = cpu.GetUse()
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
