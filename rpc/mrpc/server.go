package mrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/cpu"
	"github.com/chenjie199234/Corelib/sys/trace"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e *MsgErr)

const defaulttimeout = 200

type MrpcServer struct {
	c          *stream.InstanceConfig
	handler    map[string]func(*Msg)
	instance   *stream.Instance
	verifydata []byte
	stoptimer  *time.Timer //if this is not nil,means this server is closing
	stopch     chan struct{}
}

func NewMrpcServer(c *stream.InstanceConfig, vdata []byte) *MrpcServer {
	serverinstance := &MrpcServer{
		c:          c,
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
func (s *MrpcServer) StopMrpcServer() {
	d, _ := proto.Marshal(&Msg{
		Callid: 0,
		Error:  Errmaker(ERRCLOSING, ERRMESSAGE[ERRCLOSING]),
	})
	s.instance.SendMessageAll(d)
	s.stopch = make(chan struct{})
	s.stoptimer = time.NewTimer(time.Second)
	for {
		select {
		case <-s.stoptimer.C:
			return
		case <-s.stopch:
			s.stoptimer.Reset(time.Second)
			for len(s.stoptimer.C) > 0 {
				<-s.stoptimer.C
			}
		}
	}

}
func (s *MrpcServer) insidehandler(timeout int, handlers ...OutsideHandler) func(*Msg) {
	return func(msg *Msg) {
		defer func() {
			if e := recover(); e != nil {
				fmt.Printf("[Mrpc.server.insidehandler]panic:%s\n", e)
				msg.Metadata = nil
				msg.Cpu = cpu.GetUse()
				msg.Body = nil
				msg.Error = Errmaker(ERRPANIC, ERRMESSAGE[ERRPANIC])
			}
		}()
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
	datas := strings.Split(byte2str(peerVerifyData), "|")
	if len(datas) != 2 {
		return nil, false
	}
	if datas[1] != s.c.SelfName || hex.EncodeToString(s.verifydata) != datas[0] {
		return nil, false
	}
	return s.verifydata, true
}
func (s *MrpcServer) userfunc(p *stream.Peer, peeruniquename string, data []byte, starttime uint64) {
	go func() {
		msg := &Msg{}
		if e := proto.Unmarshal(data, msg); e != nil {
			//this is impossible
			fmt.Printf("[Mrpc.server.userfunc.impossible]unmarshal data error:%s\n", e)
			return
		}
		if s.stoptimer != nil {
			select {
			case s.stopch <- struct{}{}:
			default:
			}
			msg.Metadata = nil
			msg.Cpu = cpu.GetUse()
			msg.Body = nil
			msg.Error = Errmaker(ERRCLOSING, ERRMESSAGE[ERRCLOSING])
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, starttime); e != nil {
				fmt.Printf("[Mrpc.server.userfunc]error:%s\n", e)
			}
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
