package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/stream"
	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e *MsgErr)

const defaulttimeout = 200

type server struct {
	verifydata []byte
	instance   *stream.Instance
	lker       *sync.Mutex
	handler    map[string]OutsideHandler
	timeout    map[string]int //unit millisecond
	status     int32
}

func NewMrpcServer(c *stream.InstanceConfig, vdata []byte) *server {
	serverinstance := &server{
		verifydata: vdata,
		lker:       &sync.Mutex{},
	}
	c.Verifyfunc = serverinstance.verifyfunc
	c.Userdatafunc = serverinstance.userfunc
	serverinstance.instance = stream.NewInstance(c)
	return serverinstance
}
func (s *server) StartMrpcServer(cc *stream.TcpConfig, listenaddr string) {
	if atomic.AddInt32(&s.status, 1) > 1 {
		return
	}
	s.instance.StartTcpServer(cc, listenaddr)
}
func (s *server) RegisterHandler(path string, handler OutsideHandler) error {
	if atomic.LoadInt32(&s.status) >= 1 {
		return fmt.Errorf("[Mrpc.RegisterHandler]server already started")
	}
	s.lker.Lock()
	if _, ok := s.handler[path]; ok {
		s.lker.Unlock()
		return fmt.Errorf("[Mrpc.RegisterHandler]path:%s already registered", path)
	}
	s.handler[path] = handler
	s.lker.Unlock()
	return nil
}

func (s *server) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

func (s *server) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	go func() {
		msg := &Msg{}
		e := proto.Unmarshal(data, msg)
		if e != nil {
			fmt.Printf("[Mrpc.userfunc]unmarshal data error:%s\n", e)
			msg.Metadata = nil
			msg.Body = nil
			msg.Error = &MsgErr{
				Code: ERRDATA,
				Msg:  ERRMESSAGE[ERRDATA],
			}
			d, _ := proto.Marshal(msg)
			if e = p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
			return
		}
		handler, ok := s.handler[msg.Path]
		if !ok {
			fmt.Printf("[Mrpc.userfunc]api:%s not implement\n", msg.Path)
			msg.Metadata = nil
			msg.Body = nil
			msg.Error = &MsgErr{
				Code: ERRNOAPI,
				Msg:  ERRMESSAGE[ERRNOAPI],
			}
			d, _ := proto.Marshal(msg)
			if e = p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
			return
		}
		var ctx context.Context
		var dt time.Time
		if timeout, ok := s.timeout[msg.Path]; !ok {
			dt = time.Now().Add(defaulttimeout * time.Millisecond)
		} else {
			dt = time.Now().Add(time.Duration(timeout) * time.Millisecond)
		}
		if msg.Deadline != 0 && dt.UnixNano() > msg.Deadline {
			dt = time.Unix(0, msg.Deadline)
		}
		ctx, f := context.WithDeadline(context.Background(), dt)
		defer f()
		for k, v := range msg.Metadata {
			ctx = SetInMetadata(ctx, k, v)
		}
		resp, err := handler(ctx, msg.Body)
		if err != nil {
			msg.Body = nil
			msg.Metadata = nil
			msg.Error = err
			d, _ := proto.Marshal(msg)
			if e = p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
			return
		}
		msg.Body = resp
		msg.Error = nil
		msg.Metadata = GetAllOutMetadata(ctx)
		d, _ := proto.Marshal(msg)
		if e = p.SendMessage(d, uniqueid); e != nil {
			fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
		}
	}()
}
