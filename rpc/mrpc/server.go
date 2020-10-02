package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/cpu"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e *MsgErr)

const defaulttimeout = 200

var (
	ERRSINIT    = fmt.Errorf("[Mrpc.server]not init,call NewMrpcServer first")
	ERRSSTARTED = fmt.Errorf("[Mrpc.server]already started")
	ERRSREG     = fmt.Errorf("[Mrpc.server]already registered")
)

type Server struct {
	lker       *sync.Mutex
	handler    map[string]func(*Msg)
	instance   *stream.Instance
	verifydata []byte
}

func NewMrpcServer(c *stream.InstanceConfig, vdata []byte) *Server {
	serverinstance := &Server{
		verifydata: vdata,
		lker:       &sync.Mutex{},
	}
	c.Verifyfunc = serverinstance.verifyfunc
	c.Userdatafunc = serverinstance.userfunc
	serverinstance.instance = stream.NewInstance(c)
	return serverinstance
}
func (s *Server) StartMrpcServer(cc *stream.TcpConfig, listenaddr string) {
	s.instance.StartTcpServer(cc, listenaddr)
}
func (s *Server) insidehandler(timeout int, handlers ...OutsideHandler) func(*Msg) {
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
		for k, v := range msg.Metadata {
			ctx = SetInMetadata(ctx, k, v)
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
		msg.Metadata = GetAllOutMetadata(ctx)
	}
}
func (s *Server) RegisterHandler(path string, timeout int, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(timeout, handlers...)
}

func (s *Server) verifyfunc(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

func (s *Server) userfunc(p *stream.Peer, peeruniquename string, data []byte, starttime uint64) {
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
