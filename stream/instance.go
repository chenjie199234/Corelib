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
	peernodes    []*peernode
	stop         int32
	tcplistener  *net.TCPListener
	unixlistener *net.UnixListener
	totalpeernum int64

	noticech chan *Peer
	closech  chan struct{}

	pool *sync.Pool
}

func (this *Instance) getPeer(protot protocol, peert peertype, writebuffernum, maxmsglen uint, selfname string) *Peer {
	tempctx, tempcancel := context.WithCancel(context.Background())
	if p, ok := this.pool.Get().(*Peer); ok {
		p.reset()
		p.protocol = protot
		p.peertype = peert
		p.status = 1
		p.maxmsglen = maxmsglen
		p.Context = tempctx
		p.CancelFunc = tempcancel
		if peert == CLIENT {
			p.servername = selfname
		} else {
			p.clientname = selfname
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
		Context:         tempctx,
		CancelFunc:      tempcancel,
	}
	if peert == CLIENT {
		p.servername = selfname
	} else {
		p.clientname = selfname
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	p.CancelFunc()
	this.pool.Put(p)
}
func (this *Instance) addPeer(p *Peer) bool {
	uniquename := p.getpeeruniquename()
	node := this.peernodes[this.getindex(uniquename)]
	node.Lock()
	if _, ok := node.peers[uniquename]; ok {
		node.Unlock()
		return false
	}
	p.parentnode = node
	node.peers[uniquename] = p
	atomic.AddInt64(&this.totalpeernum, 1)
	node.Unlock()
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
		selfname:  group + "." + name,
		c:         c,
		peernodes: make([]*peernode, c.GroupNum),
		stop:      0,
		noticech:  make(chan *Peer, 1024),
		closech:   make(chan struct{}, 1),
		pool:      &sync.Pool{},
	}
	for i := range stream.peernodes {
		stream.peernodes[i] = &peernode{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peernodes[i])
	}
	go func() {
		for {
			p := <-stream.noticech
			if p != nil {
				if p.parentnode != nil {
					p.parentnode.Lock()
					atomic.AddInt64(&stream.totalpeernum, -1)
					delete(p.parentnode.peers, p.getpeeruniquename())
					p.parentnode.Unlock()
				}
				stream.putPeer(p)
			}
			if atomic.LoadInt32(&stream.stop) == 1 {
				count := 0
				for _, node := range stream.peernodes {
					node.RLock()
					count += len(node.peers)
					node.RUnlock()
				}
				if count == 0 {
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
	for _, node := range this.peernodes {
		node.RLock()
		for _, peer := range node.peers {
			peer.Close()
		}
		node.RUnlock()
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
	for _, node := range this.peernodes {
		node.RWMutex.RLock()
		for _, peer := range node.peers {
			wg.Add(1)
			go func(p *Peer) {
				p.SendMessage(data, p.starttime, block)
				wg.Done()
			}(peer)
		}
		node.RWMutex.RUnlock()
	}
	wg.Wait()
}

func (this *Instance) heart(node *peernode) {
	tker := time.NewTicker(time.Duration(this.c.HeartprobeInterval) * time.Millisecond)
	for {
		<-tker.C
		now := uint64(time.Now().UnixNano())
		node.RLock()
		for _, p := range node.peers {
			if p.status == 0 {
				continue
			}
			templastactive := atomic.LoadUint64(&p.lastactive)
			temprecvidlestart := atomic.LoadUint64(&p.recvidlestart)
			tempsendidlestart := atomic.LoadUint64(&p.sendidlestart)
			if now >= templastactive && now-templastactive > uint64(this.c.HeartbeatTimeout) {
				//heartbeat timeout
				log.Error("[Stream.heart] heartbeat timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename())
				p.closeconn()
				continue
			}
			if now >= tempsendidlestart && now-tempsendidlestart > uint64(this.c.SendIdleTimeout) {
				//send idle timeout
				log.Error("[Stream.heart] send idle timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename())
				p.closeconn()
				continue
			}
			if this.c.RecvIdleTimeout != 0 && now >= temprecvidlestart && now-temprecvidlestart > uint64(this.c.RecvIdleTimeout) {
				//recv idle timeout
				log.Error("[Stream.heart] recv idle timeout", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename())
				p.closeconn()
				continue
			}
			//send heart beat data
			data := makeHeartMsg(true)
			select {
			case p.heartbeatbuffer <- data:
			default:
				log.Error("[Stream.heart] to", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename(), "error: heart buffer full")
				bufpool.PutBuffer(data)
			}
		}
		node.RUnlock()
	}
}
func (this *Instance) getindex(peername string) uint {
	return uint(common.BkdrhashString(peername, uint64(this.c.GroupNum)))
}
