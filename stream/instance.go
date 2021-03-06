package stream

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

type Instance struct {
	selfname     string
	c            *InstanceConfig
	peergroups   []*peergroup
	stop         int32
	tcplistener  *net.TCPListener
	unixlistener *net.UnixListener
	totalpeernum int64

	noticech chan *Peer
	closech  chan struct{}

	pool *sync.Pool
}

func (this *Instance) getPeer(writebuffernum, maxmsglen uint) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	if p, ok := this.pool.Get().(*Peer); ok {
		p.maxmsglen = maxmsglen
		p.Context = ctx
		p.CancelFunc = cancel
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
		return p
	}
	p := &Peer{
		maxmsglen:      maxmsglen,
		writerbuffer:   make(chan *bufpool.Buffer, writebuffernum),
		pingpongbuffer: make(chan *bufpool.Buffer, 2),
		Context:        ctx,
		CancelFunc:     cancel,
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	p.CancelFunc()
	p.parentgroup = nil
	p.peername = ""
	p.sid = 0
	p.maxmsglen = 0
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
	p.fd = 0
	p.lastactive = 0
	p.recvidlestart = 0
	p.sendidlestart = 0
	p.data = nil
	this.pool.Put(p)
}
func (this *Instance) addPeer(p *Peer) bool {
	uniquename := p.getUniqueName()
	group := this.peergroups[common.BkdrhashString(uniquename, uint64(this.c.GroupNum))]
	group.Lock()
	if _, ok := group.peers[uniquename]; ok {
		group.Unlock()
		return false
	}
	p.parentgroup = group
	group.peers[uniquename] = p
	atomic.AddInt64(&this.totalpeernum, 1)
	group.Unlock()
	return true
}
func (this *Instance) delPeer(p *Peer) {
	uniquename := p.getUniqueName()
	p.parentgroup.Lock()
	if _, ok := p.parentgroup.peers[uniquename]; ok {
		delete(p.parentgroup.peers, uniquename)
		atomic.AddInt64(&this.totalpeernum, -1)
	}
	p.parentgroup.Unlock()
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
		selfname:   group + "." + name,
		c:          c,
		peergroups: make([]*peergroup, c.GroupNum),
		stop:       0,
		noticech:   make(chan *Peer, 1024),
		closech:    make(chan struct{}),
		pool:       &sync.Pool{},
	}
	for i := range stream.peergroups {
		stream.peergroups[i] = &peergroup{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peergroups[i], i)
	}
	go func() {
		defer close(stream.closech)
		for {
			p := <-stream.noticech
			if p != nil {
				stream.delPeer(p)
				stream.putPeer(p)
			}
			if atomic.LoadInt32(&stream.stop) == 1 && stream.totalpeernum == 0 {
				return
			}
		}
	}()
	return stream, nil
}
func (this *Instance) Stop() {
	if atomic.SwapInt32(&this.stop, 1) == 1 {
		return
	}
	if this.tcplistener != nil {
		this.tcplistener.Close()
	}
	if this.unixlistener != nil {
		this.unixlistener.Close()
	}
	for _, group := range this.peergroups {
		group.RLock()
		for _, peer := range group.peers {
			peer.Close(peer.sid)
		}
		group.RUnlock()
	}
	//prevent notice block on empty chan
	select {
	case this.noticech <- nil:
	default:
	}
	<-this.closech
}
func (this *Instance) GetSelfName() string {
	return this.selfname
}
func (this *Instance) SendMessageAll(data []byte, block bool) {
	wg := &sync.WaitGroup{}
	for _, group := range this.peergroups {
		group.RWMutex.RLock()
		for _, peer := range group.peers {
			wg.Add(1)
			go func(p *Peer) {
				p.SendMessage(data, p.sid, block)
				wg.Done()
			}(peer)
		}
		group.RWMutex.RUnlock()
	}
	wg.Wait()
}

func (this *Instance) heart(group *peergroup, index int) {
	check := func() {
		now := time.Now().UnixNano()
		group.RLock()
		for _, p := range group.peers {
			if p.sid <= 0 {
				continue
			}
			templastactive := atomic.LoadInt64(&p.lastactive)
			temprecvidlestart := atomic.LoadInt64(&p.recvidlestart)
			tempsendidlestart := atomic.LoadInt64(&p.sendidlestart)
			if now >= templastactive && now-templastactive > int64(this.c.HeartbeatTimeout) {
				//heartbeat timeout
				log.Error("[Stream.heart] heartbeat timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			if now >= tempsendidlestart && now-tempsendidlestart > int64(this.c.SendIdleTimeout) {
				//send idle timeout
				log.Error("[Stream.heart] send idle timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			if this.c.RecvIdleTimeout != 0 && now >= temprecvidlestart && now-temprecvidlestart > int64(this.c.RecvIdleTimeout) {
				//recv idle timeout
				log.Error("[Stream.heart] recv idle timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			//send heart beat data
			data := makePingMsg(nil)
			select {
			case p.pingpongbuffer <- data:
			default:
				log.Error("[Stream.heart] to:", p.getUniqueName(), "error: heart buffer full")
				bufpool.PutBuffer(data)
			}
		}
		group.RUnlock()
	}
	var tker *time.Ticker
	delay := int64(float64(index) / float64(this.c.GroupNum) * float64(this.c.HeartprobeInterval.Nanoseconds()))
	if delay != 0 {
		time.Sleep(time.Duration(delay))
		tker = time.NewTicker(this.c.HeartprobeInterval)
		check()
	} else {
		tker = time.NewTicker(this.c.HeartprobeInterval)
	}
	for {
		<-tker.C
		check()
	}
}
