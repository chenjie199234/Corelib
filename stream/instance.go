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

func (this *Instance) getPeer(protot protocol, peert peertype, writebuffernum, maxmsglen uint, selfname string) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	if p, ok := this.pool.Get().(*Peer); ok {
		p.protocol = protot
		p.peertype = peert
		p.status = 1
		p.maxmsglen = maxmsglen
		p.Context = ctx
		p.CancelFunc = cancel
		if peert == CLIENT {
			p.servername = selfname
		} else {
			p.clientname = selfname
		}
		for len(p.writerbuffer) > 0 {
			if v := <-p.writerbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		for len(p.heartbeatbuffer) > 0 {
			if v := <-p.heartbeatbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		return p
	}
	p := &Peer{
		peertype:        peert,
		protocol:        protot,
		status:          1,
		maxmsglen:       maxmsglen,
		writerbuffer:    make(chan *bufpool.Buffer, writebuffernum),
		heartbeatbuffer: make(chan *bufpool.Buffer, 1),
		Context:         ctx,
		CancelFunc:      cancel,
	}
	if peert == CLIENT {
		p.servername = selfname
	} else {
		p.clientname = selfname
	}
	for len(p.writerbuffer) > 0 {
		if v := <-p.writerbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	for len(p.heartbeatbuffer) > 0 {
		if v := <-p.heartbeatbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	p.CancelFunc()
	p.parentgroup = nil
	p.clientname = ""
	p.servername = ""
	p.peertype = 0
	p.protocol = 0
	p.starttime = 0
	p.closeread = true
	p.closewrite = true
	p.status = 0
	p.maxmsglen = 0
	for len(p.writerbuffer) > 0 {
		if v := <-p.writerbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	for len(p.heartbeatbuffer) > 0 {
		if v := <-p.heartbeatbuffer; v != nil {
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
	group := this.peergroups[this.getindex(uniquename)]
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
		closech:    make(chan struct{}, 1),
		pool:       &sync.Pool{},
	}
	for i := range stream.peergroups {
		stream.peergroups[i] = &peergroup{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peergroups[i])
	}
	go func() {
		for {
			p := <-stream.noticech
			if p != nil {
				if p.parentgroup != nil {
					p.parentgroup.Lock()
					delete(p.parentgroup.peers, p.getUniqueName())
					atomic.AddInt64(&stream.totalpeernum, -1)
					p.parentgroup.Unlock()
				}
				stream.putPeer(p)
			}
			if atomic.LoadInt32(&stream.stop) == 1 {
				finish := true
				for _, group := range stream.peergroups {
					group.RLock()
					if len(group.peers) != 0 {
						finish = false
						group.RUnlock()
						break
					}
					group.RUnlock()
				}
				if finish {
					select {
					case stream.closech <- struct{}{}:
					default:
					}
				}
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
			peer.Close()
		}
		group.RUnlock()
	}
	//prevent notice block on empty chan
	this.noticech <- (*Peer)(nil)
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
				p.SendMessage(data, p.starttime, block)
				wg.Done()
			}(peer)
		}
		group.RWMutex.RUnlock()
	}
	wg.Wait()
}

func (this *Instance) heart(group *peergroup) {
	tker := time.NewTicker(time.Duration(this.c.HeartprobeInterval) * time.Millisecond)
	for {
		<-tker.C
		now := uint64(time.Now().UnixNano())
		group.RLock()
		for _, p := range group.peers {
			if p.status == 0 {
				continue
			}
			templastactive := atomic.LoadUint64(&p.lastactive)
			temprecvidlestart := atomic.LoadUint64(&p.recvidlestart)
			tempsendidlestart := atomic.LoadUint64(&p.sendidlestart)
			if now >= templastactive && now-templastactive > uint64(this.c.HeartbeatTimeout) {
				//heartbeat timeout
				log.Error("[Stream.heart] heartbeat timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName())
				p.closeconn()
				continue
			}
			if now >= tempsendidlestart && now-tempsendidlestart > uint64(this.c.SendIdleTimeout) {
				//send idle timeout
				log.Error("[Stream.heart] send idle timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName())
				p.closeconn()
				continue
			}
			if this.c.RecvIdleTimeout != 0 && now >= temprecvidlestart && now-temprecvidlestart > uint64(this.c.RecvIdleTimeout) {
				//recv idle timeout
				log.Error("[Stream.heart] recv idle timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName())
				p.closeconn()
				continue
			}
			//send heart beat data
			data := makeHeartMsg(true)
			select {
			case p.heartbeatbuffer <- data:
			default:
				log.Error("[Stream.heart] to", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName(), "error: heart buffer full")
				bufpool.PutBuffer(data)
			}
		}
		group.RUnlock()
	}
}
func (this *Instance) getindex(peername string) uint {
	return uint(common.BkdrhashString(peername, uint64(this.c.GroupNum)))
}
