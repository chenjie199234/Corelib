package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/cpu"
	"github.com/chenjie199234/Corelib/sys/trace"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e *MsgErr)

const defaulttimeout = 200

var (
	ERRSINIT    = fmt.Errorf("[Mrpc.server]not init,call NewMrpcServer first")
	ERRSSTARTED = fmt.Errorf("[Mrpc.server]already started")
	ERRSREG     = fmt.Errorf("[Mrpc.server]already registered")
)

type MrpcServer struct {
	c          *stream.InstanceConfig
	lker       *sync.Mutex
	handler    map[string]func(*Msg)
	instance   *stream.Instance
	verifydata []byte
}

func NewMrpcServer(c *stream.InstanceConfig, vdata []byte) *MrpcServer {
	serverinstance := &MrpcServer{
		c:          c,
		lker:       &sync.Mutex{},
		handler:    make(map[string]func(*Msg), 10),
		verifydata: vdata,
	}
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = nil
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = nil
	serverinstance.instance = stream.NewInstance(&dupc)
	return serverinstance
}
func (s *MrpcServer) StartMrpcServer(cc *stream.TcpConfig, listenaddr string) {
	s.instance.StartTcpServer(cc, listenaddr)
}
func (s *MrpcServer) insidehandler(timeout int, handlers ...OutsideHandler) func(*Msg) {
	return func(msg *Msg) {
		var ctx context.Context
		var dl time.Time
		if timeout == 0 {
			dl = time.Now().Add(defaulttimeout * time.Millisecond)
		} else {
			dl = time.Now().Add(time.Duration(timeout) * time.Millisecond)
		}
		if msg.Deadline != 0 && dl.UnixNano() > msg.Deadline {
			dl = time.Unix(0, msg.Deadline)
		}
		ctx, f := context.WithDeadline(context.Background(), dl)
		defer f()
		if len(msg.Metadata) > 0 {
			ctx = SetAllMetadata(ctx, msg.Metadata)
		}
		if msg.Trace == "" {
			ctx = trace.SetTrace(ctx, trace.MakeTrace())
		} else {
			ctx = trace.SetTrace(ctx, msg.Trace)
		}
		var resp []byte
		var err *MsgErr
		for _, handler := range handlers {
			resp, err = handler(ctx, msg.Body)
			if err != nil {
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
				msg.Error = err
				msg.Metadata = nil
				return
			}
		}
		msg.Deadline = 0
		msg.Body = resp
		msg.Cpu = cpu.GetUse()
		msg.Error = nil
		msg.Metadata = nil
	}
}
func (s *MrpcServer) RegisterHandler(path string, timeout int, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(timeout, handlers...)
}

func (s *MrpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

func (s *MrpcServer) userfunc(p *stream.Peer, peeruniquename string, data []byte, starttime uint64) {
	go func() {
		defer func() {
			if e := recover(); e != nil {
				fmt.Printf("[Mrpc.server.userfunc]panic:%s\n", e)
			}
		}()
		msg := &Msg{}
		if e := proto.Unmarshal(data, msg); e != nil {
			//this is impossible
			fmt.Printf("[Mrpc.server.userfunc.impossible]unmarshal data error:%s\n", e)
			return
		}
		handler, ok := s.handler[msg.Path]
		if !ok {
			fmt.Printf("[Mrpc.server.userfunc]api:%s not implement\n", msg.Path)
			msg.Metadata = nil
			msg.Cpu = cpu.GetUse()
			msg.Body = nil
			msg.Error = Errmaker(ERRNOAPI, ERRMESSAGE[ERRNOAPI])
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, starttime); e != nil {
				fmt.Printf("[Mrpc.server.userfunc]error:%s\n", e)
			}
			return
		}
		handler(msg)
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(d, starttime); e != nil {
			fmt.Printf("[Mrpc.server.userfunc]error:%s\n", e)
		}
	}()
}
