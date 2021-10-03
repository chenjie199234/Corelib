package stream

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/util/common"
)

type Instance struct {
	selfname string
	c        *InstanceConfig

	sync.Mutex
	tcplistener net.Listener
	mng         *connmng

	noticech  chan *Peer
	closewait *sync.WaitGroup

	pool *sync.Pool
}

func (this *Instance) getPeer(writebuffernum, maxmsglen uint32) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	if p, ok := this.pool.Get().(*Peer); ok {
		p.selfmaxmsglen = maxmsglen
		for len(p.writerbuffer) > 0 {
			if v := <-p.writerbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		for len(p.pingpongbuffer) > 0 {
			if v := <-p.pingpongbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		p.Context = ctx
		p.CancelFunc = cancel
		return p
	}
	p := &Peer{
		selfmaxmsglen:  maxmsglen,
		writerbuffer:   make(chan *bufpool.Buffer, writebuffernum),
		pingpongbuffer: make(chan *bufpool.Buffer, 2),
		Context:        ctx,
		CancelFunc:     cancel,
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	p.CancelFunc()
	p.peername = ""
	p.sid = 0
	p.selfmaxmsglen = 0
	p.peermaxmsglen = 0
	for len(p.writerbuffer) > 0 {
		if v := <-p.writerbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	for len(p.pingpongbuffer) > 0 {
		if v := <-p.pingpongbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	p.conn = nil
	p.lastactive = 0
	p.recvidlestart = 0
	p.sendidlestart = 0
	p.data = nil
	this.pool.Put(p)
}

//be careful about the callback func race
func NewInstance(c *InstanceConfig, group, name string) (*Instance, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group+"."+name, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		return nil, errors.New("[Stream.NewInstance] config is nil")
	}
	//verify func can't be nill
	//user data deal func can't be nill
	//online and offline func can be nill
	if c.Verifyfunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing verify function")
	}
	if c.Userdatafunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing userdata function")
	}
	c.validate()
	stream := &Instance{
		selfname:  group + "." + name,
		c:         c,
		mng:       newconnmng(int(c.GroupNum), c.HeartbeatTimeout, c.HeartprobeInterval, c.SendIdleTimeout, c.RecvIdleTimeout),
		noticech:  make(chan *Peer, 1024),
		closewait: &sync.WaitGroup{},
		pool:      &sync.Pool{},
	}
	stream.closewait.Add(1)
	go func() {
		for {
			p := <-stream.noticech
			if p != nil {
				stream.mng.DelPeer(p)
				stream.putPeer(p)
			}
			if stream.mng.Finished() {
				stream.closewait.Done()
				return
			}
		}
	}()
	return stream, nil
}
func (this *Instance) Stop() {
	if this.mng.stop() {
		if this.tcplistener != nil {
			this.tcplistener.Close()
		}
		//prevent notice block on empty chan
		select {
		case this.noticech <- nil:
		default:
		}
	}
	this.closewait.Wait()
}
func (this *Instance) GetSelfName() string {
	return this.selfname
}
func (this *Instance) GetPeerNum() int32 {
	return this.mng.GetPeerNum()
}
func (this *Instance) SendMessageAll(data []byte, block bool) {
	this.mng.SendMessage(data, block)
}
