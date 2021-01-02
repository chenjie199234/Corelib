package stream

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/common"
)

type Instance struct {
	conf         *InstanceConfig
	peernodes    []*peernode
	stop         int64
	tcplistener  *net.TCPListener
	unixlistener *net.UnixListener
	webserver    *http.Server

	tcpPool  *sync.Pool
	unixPool *sync.Pool
	webPool  *sync.Pool
}

func (this *Instance) getPeer(protot, peert, writebuffernum, maxmsglen int, selfname *string) *Peer {
	tempctx, tempcancel := context.WithCancel(context.Background())
	switch protot {
	case TCP:
		if p, ok := this.tcpPool.Get().(*Peer); ok {
			p.reset()
			p.protocoltype = TCP
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
	case UNIXSOCKET:
		if p, ok := this.unixPool.Get().(*Peer); ok {
			p.reset()
			p.protocoltype = UNIXSOCKET
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
	case WEBSOCKET:
		if p, ok := this.webPool.Get().(*Peer); ok {
			p.reset()
			p.protocoltype = WEBSOCKET
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
	}
	p := &Peer{
		parentnode:      nil,
		clientname:      nil,
		servername:      nil,
		peertype:        peert,
		protocoltype:    protot,
		starttime:       0,
		status:          1,
		maxmsglen:       maxmsglen,
		writerbuffer:    make(chan []byte, writebuffernum),
		heartbeatbuffer: make(chan []byte, 3),
		conn:            nil,
		lastactive:      0,
		recvidlestart:   0,
		sendidlestart:   0,
		Context:         tempctx,
		CancelFunc:      tempcancel,
		data:            nil,
	}
	if peert == CLIENT {
		p.servername = selfname
	} else {
		p.clientname = selfname
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	tempprotocoltype := p.protocoltype
	p.reset()
	switch tempprotocoltype {
	case TCP:
		this.tcpPool.Put(p)
	case UNIXSOCKET:
		this.unixPool.Put(p)
	case WEBSOCKET:
		this.webPool.Put(p)
	}
}
func (this *Instance) addPeer(p *Peer) bool {
	uniquename := p.getpeeruniquename()
	node := this.peernodes[this.getindex(uniquename)]
	node.Lock()
	if _, ok := node.peers[uniquename]; ok {
		p.closeconn()
		this.putPeer(p)
		node.Unlock()
		return false
	}
	p.parentnode = node
	node.peers[uniquename] = p
	node.Unlock()
	return true
}

//be careful about the callback func race
func NewInstance(c *InstanceConfig) *Instance {
	if e := checkInstanceConfig(c); e != nil {
		panic(e)
	}
	stream := &Instance{
		conf:      c,
		peernodes: make([]*peernode, c.GroupNum),
		stop:      0,

		tcpPool:  &sync.Pool{},
		unixPool: &sync.Pool{},
		webPool:  &sync.Pool{},
	}
	for i := range stream.peernodes {
		stream.peernodes[i] = &peernode{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peernodes[i])
	}
	return stream
}
func (this *Instance) Stop() {
	if atomic.SwapInt64(&this.stop, 1) == 1 {
		return
	}
	if this.tcplistener != nil {
		this.tcplistener.Close()
	}
	if this.unixlistener != nil {
		this.unixlistener.Close()
	}
	if this.webserver != nil {
		this.webserver.Shutdown(context.Background())
	}
	for _, node := range this.peernodes {
		node.RLock()
		for _, peer := range node.peers {
			peer.closeconn()
		}
		node.RUnlock()
	}
}

func (this *Instance) SendMessageAll(data []byte, block bool) {
	for _, node := range this.peernodes {
		node.RWMutex.RLock()
		for _, peer := range node.peers {
			peer.SendMessage(data, peer.starttime, block)
		}
		node.RWMutex.RUnlock()
	}
}

func (this *Instance) heart(node *peernode) {
	tker := time.NewTicker(time.Duration(this.conf.HeartprobeInterval) * time.Millisecond)
	for {
		<-tker.C
		now := uint64(time.Now().UnixNano())
		node.RLock()
		for _, p := range node.peers {
			if p.status == 0 {
				continue
			}
			templastactive := p.lastactive
			temprecvidlestart := p.recvidlestart
			tempsendidlestart := p.sendidlestart
			if now >= templastactive && now-templastactive > this.conf.HeartbeatTimeout*1000*1000 {
				//heartbeat timeout
				fmt.Printf("[Stream.%s.heart] heart timeout %s:%s addr:%s\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
				p.closeconn()
				continue
			}
			if now >= tempsendidlestart && now-tempsendidlestart > this.conf.SendIdleTimeout*1000*1000 {
				//send idle timeout
				fmt.Printf("[Stream.%s.heart] send idle timeout %s:%s addr:%s\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
				p.closeconn()
				continue
			}
			if this.conf.RecvIdleTimeout != 0 && now >= temprecvidlestart && now-temprecvidlestart > this.conf.RecvIdleTimeout*1000*1000 {
				//recv idle timeout
				fmt.Printf("[Stream.%s.heart] recv idle timeout %s:%s addr:%s\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
				p.closeconn()
				continue
			}
			//send heart beat data
			var data []byte
			switch p.protocoltype {
			case TCP:
				fallthrough
			case UNIXSOCKET:
				data = makeHeartMsg(true)
			case WEBSOCKET:
				data = makeHeartMsg(false)
			}
			select {
			case p.heartbeatbuffer <- data:
			default:
				fmt.Printf("[Stream.%s.heart] send heart msg to %s:%s addr:%s failed:heart buffer is full\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
			}
		}
		node.RUnlock()
	}
}
func (this *Instance) getindex(peername string) uint {
	return uint(common.BkdrhashString(peername, uint64(this.conf.GroupNum)))
}
