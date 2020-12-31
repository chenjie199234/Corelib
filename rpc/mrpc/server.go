package mrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/cpu"
	//"github.com/chenjie199234/Corelib/sys/trace"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e error)

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
		handler:    make(map[string]func(*Msg), 10),
		verifydata: vdata,
	}
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = nil
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = nil
	serverinstance.c = &dupc
	serverinstance.instance = stream.NewInstance(&dupc)
	return serverinstance
}
func (s *MrpcServer) StartMrpcServer(listenaddr string) {
	s.instance.StartTcpServer(listenaddr)
}
func (s *MrpcServer) StopMrpcServer() {
	d, _ := proto.Marshal(&Msg{
		Callid: 0,
		Error:  ERR[ERRCLOSING].Error(),
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
				msg.Path = ""
				msg.Trace = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
				msg.Error = ERR[ERRPANIC].Error()
				msg.Metadata = nil
			}
		}()
		ctx := context.Background()
		now := time.Now()
		var dl time.Time
		if timeout != 0 {
			if msg.Deadline == 0 || now.UnixNano()+int64(timeout) <= msg.Deadline {
				dl = now.Add(time.Duration(timeout) * time.Millisecond)
			} else {
				dl = time.Unix(0, msg.Deadline)
			}
		} else if msg.Deadline != 0 {
			dl = time.Unix(0, msg.Deadline)
		}
		if !dl.IsZero() {
			if dl.UnixNano() <= (now.UnixNano() + int64(time.Millisecond)) {
				msg.Path = ""
				msg.Trace = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
				msg.Error = ERR[ERRCTXTIMEOUT].Error()
				msg.Metadata = nil
				return
			}
			var f context.CancelFunc
			ctx, f = context.WithDeadline(ctx, dl)
			defer f()
		}
		if len(msg.Metadata) > 0 {
			ctx = SetAllMetadata(ctx, msg.Metadata)
		}
		//if msg.Trace == "" {
		//        ctx = trace.SetTrace(ctx, trace.MakeTrace())
		//} else {
		//        ctx = trace.SetTrace(ctx, msg.Trace)
		//}
		var resp []byte
		var err error
		for _, handler := range handlers {
			resp, err = handler(ctx, msg.Body)
			if err != nil {
				msg.Path = ""
				msg.Trace = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Cpu = cpu.GetUse()
				msg.Error = err.Error()
				msg.Metadata = nil
				return
			}
		}
		msg.Path = ""
		msg.Trace = ""
		msg.Deadline = 0
		msg.Body = resp
		msg.Cpu = cpu.GetUse()
		msg.Error = ""
		msg.Metadata = nil
	}
}
func (s *MrpcServer) RegisterHandler(path string, timeout int, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(timeout, handlers...)
}
func (s *MrpcServer) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	datas := strings.Split(common.Byte2str(peerVerifyData), "|")
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
			msg.Error = ERR[ERRCLOSING].Error()
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, starttime); e != nil {
				fmt.Printf("[Mrpc.server.userfunc]error:%s\n", e)
			}
			return
		}
		handler, ok := s.handler[msg.Path]
		if !ok {
			fmt.Printf("[Mrpc.server.userfunc]api:%s not implement\n", msg.Path)
			msg.Path = ""
			msg.Trace = ""
			msg.Metadata = nil
			msg.Cpu = cpu.GetUse()
			msg.Body = nil
			msg.Error = ERR[ERRNOAPI].Error()
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
