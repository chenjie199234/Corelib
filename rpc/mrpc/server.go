package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/stream"
	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(ctx context.Context, req []byte) (resp []byte, e *MsgErr)

const defaulttimeout = 200

var (
	ERRSINIT    = fmt.Errorf("[Mrpc.server]not init,call NewMrpcServer first")
	ERRSSTARTED = fmt.Errorf("[Mrpc.server]already started")
	ERRSREG     = fmt.Errorf("[Mrpc.server]already registered")
)

type server struct {
	lker       *sync.Mutex
	handler    map[string]OutsideHandler
	timeout    map[string]int //unit millisecond
	instance   *stream.Instance
	verifydata []byte
	status     int32
}

var serverinstance *server

func NewMrpcServer(c *stream.InstanceConfig, vdata []byte) {
	if serverinstance != nil {
		return
	}
	serverinstance = &server{
		verifydata: vdata,
		lker:       &sync.Mutex{},
	}
	c.Verifyfunc = serverinstance.verifyfunc
	c.Userdatafunc = serverinstance.userfunc
	serverinstance.instance = stream.NewInstance(c)
	return
}
func StartMrpcServer(cc *stream.TcpConfig, listenaddr string) error {
	if serverinstance == nil {
		return ERRSINIT
	}
	serverinstance.lker.Lock()
	if serverinstance.status >= 1 {
		serverinstance.lker.Unlock()
		return ERRSSTARTED
	}
	serverinstance.status = 1
	serverinstance.lker.Unlock()
	serverinstance.instance.StartTcpServer(cc, listenaddr)
	return nil
}
func RegisterHandler(path string, handler OutsideHandler, timeout int) error {
	serverinstance.lker.Lock()
	if serverinstance.status >= 1 {
		serverinstance.lker.Unlock()
		return ERRSSTARTED
	}
	if _, ok := serverinstance.handler[path]; ok {
		serverinstance.lker.Unlock()
		return ERRSREG
	}
	if _, ok := serverinstance.timeout[path]; ok {
		serverinstance.lker.Unlock()
		return ERRSREG
	}
	serverinstance.handler[path] = handler
	serverinstance.timeout[path] = timeout
	serverinstance.lker.Unlock()
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
		if e := proto.Unmarshal(data, msg); e != nil {
			//this is impossible
			fmt.Printf("[Mrpc.userfunc.impossible]unmarshal data error:%s\n", e)
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
			if e := p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
			return
		}
		var ctx context.Context
		var dl time.Time
		if timeout, ok := s.timeout[msg.Path]; !ok || timeout == 0 {
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
		resp, err := handler(ctx, msg.Body)
		if err != nil {
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = err
			msg.Metadata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
		} else {
			msg.Deadline = 0
			msg.Body = resp
			msg.Error = nil
			msg.Metadata = GetAllOutMetadata(ctx)
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(d, uniqueid); e != nil {
				fmt.Printf("[Mrpc.userfunc]error:%s\n", e)
			}
		}
	}()
}
