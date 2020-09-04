package stream

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/buffer"
)

func (this *Instance) SendMessage(peername string, userdata []byte) error {
	node := this.peernodes[this.getindex(peername)]
	node.RLock()
	p, ok := node.peers[peername]
	if !ok {
		node.RUnlock()
		return ERRCONNCLOSED
	}
	var data []byte
	msg := &userMsg{
		uniqueid: p.starttime,
		sender:   p.getselfname(),
		userdata: userdata,
	}
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		data = makeUserMsg(msg, true)
	case WEBSOCKET:
		data = makeUserMsg(msg, false)
	}
	select {
	case p.writerbuffer <- data:
	default:
		node.RUnlock()
		return ERRFULL
	}
	node.RUnlock()
	return nil
}

//unit nanosecond
func (this *Instance) GetAverageNetLag(peername string) (uint64, error) {
	node := this.peernodes[this.getindex(peername)]
	node.RLock()
	p, ok := node.peers[peername]
	if !ok {
		node.RUnlock()
		return 0, ERRCONNCLOSED
	}
	lag := p.GetAverageNetLag()
	node.RUnlock()
	return lag, nil
}

//unit nanosecond
func (this *Instance) GetPeekNetLag(peername string) (uint64, error) {
	node := this.peernodes[this.getindex(peername)]
	node.RLock()
	p, ok := node.peers[peername]
	if !ok {
		node.RUnlock()
		return 0, ERRCONNCLOSED
	}
	lag := p.GetPeekNetLag()
	node.RUnlock()
	return lag, nil
}

func (this *Instance) Close(peername string) error {
	node := this.peernodes[this.getindex(peername)]
	node.RLock()
	p, ok := node.peers[peername]
	if !ok {
		node.RUnlock()
		return nil
	}
	p.closeconn()
	p.status = false
	node.RUnlock()
	return nil
}

type Instance struct {
	conf      *InstanceConfig
	status    bool //true-working,false-closing
	peernodes []*peernode

	peerPool          *sync.Pool
	websocketPeerPool *sync.Pool
}

func (this *Instance) getPeer(t int, conf unsafe.Pointer) *Peer {
	switch t {
	case TCP:
		if p, ok := this.peerPool.Get().(*Peer); ok {
			if len(p.writerbuffer) > 0 {
				<-p.writerbuffer
			}
			if len(p.heartbeatbuffer) > 0 {
				<-p.heartbeatbuffer
			}
			return p
		}
		tempctx, tempcancel := context.WithCancel(context.Background())
		c := (*TcpConfig)(conf)
		return &Peer{
			clientname:      "",
			servername:      "",
			peertype:        0,
			protocoltype:    0,
			starttime:       0,
			readbuffer:      buffer.NewBuf(c.AppMinReadBufferLen, c.AppMaxReadBufferLen),
			tempbuffer:      make([]byte, c.AppMinReadBufferLen),
			tempbuffernum:   0,
			writerbuffer:    make(chan []byte, c.AppWriteBufferNum),
			heartbeatbuffer: make(chan []byte, 3),
			conn:            nil,
			lastactive:      0,
			netlag:          make([]uint64, this.conf.NetLagSampleNum),
			netlagindex:     0,
			status:          false,
			ctx:             tempctx,
			cancel:          tempcancel,
		}
	case UNIXSOCKET:
		if p, ok := this.peerPool.Get().(*Peer); ok {
			if len(p.writerbuffer) > 0 {
				<-p.writerbuffer
			}
			if len(p.heartbeatbuffer) > 0 {
				<-p.heartbeatbuffer
			}
			return p
		}
		tempctx, tempcancel := context.WithCancel(context.Background())
		c := (*UnixConfig)(conf)
		return &Peer{
			clientname:      "",
			servername:      "",
			peertype:        0,
			protocoltype:    0,
			starttime:       0,
			readbuffer:      buffer.NewBuf(c.AppMinReadBufferLen, c.AppMaxReadBufferLen),
			tempbuffer:      make([]byte, c.AppMinReadBufferLen),
			tempbuffernum:   0,
			writerbuffer:    make(chan []byte, c.AppWriteBufferNum),
			heartbeatbuffer: make(chan []byte, 3),
			conn:            nil,
			lastactive:      0,
			netlag:          make([]uint64, this.conf.NetLagSampleNum),
			netlagindex:     0,
			status:          false,
			ctx:             tempctx,
			cancel:          tempcancel,
		}
	case WEBSOCKET:
		if p, ok := this.websocketPeerPool.Get().(*Peer); ok {
			if len(p.writerbuffer) > 0 {
				<-p.writerbuffer
			}
			if len(p.heartbeatbuffer) > 0 {
				<-p.heartbeatbuffer
			}
			return p
		}
		tempctx, tempcancel := context.WithCancel(context.Background())
		c := (*WebConfig)(conf)
		return &Peer{
			clientname:      "",
			servername:      "",
			peertype:        0,
			protocoltype:    0,
			starttime:       0,
			readbuffer:      nil,
			tempbuffer:      nil,
			tempbuffernum:   0,
			writerbuffer:    make(chan []byte, c.AppWriteBufferNum),
			heartbeatbuffer: make(chan []byte, 3),
			conn:            nil,
			lastactive:      0,
			netlag:          make([]uint64, this.conf.NetLagSampleNum),
			netlagindex:     0,
			status:          false,
			ctx:             tempctx,
			cancel:          tempcancel,
		}
	default:
		return nil
	}
}
func (this *Instance) putPeer(p *Peer) {
	p.cancel()
	p.cancel = nil
	p.ctx = nil
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	p.peertype = 0
	p.protocoltype = 0
	p.starttime = 0
	p.tempbuffernum = 0
	if len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	if len(p.heartbeatbuffer) > 0 {
		<-p.heartbeatbuffer
	}
	p.closeconn()
	p.lastactive = 0
	for i := range p.netlag {
		p.netlag[i] = 0
	}
	p.netlagindex = 0
	p.status = false
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		p.readbuffer.Reset()
		this.peerPool.Put(p)
	case WEBSOCKET:
		this.websocketPeerPool.Put(p)
	}
}
func (this *Instance) addPeer(p *Peer) bool {
	node := this.peernodes[this.getindex(p.getpeername())]
	node.Lock()
	if _, ok := node.peers[p.getpeername()]; ok {
		p.closeconn()
		this.putPeer(p)
		node.Unlock()
		return false
	}
	p.parentnode = node
	node.peers[p.getpeername()] = p
	node.Unlock()
	return true
}
func NewInstance(c *InstanceConfig) *Instance {
	if e := checkInstanceConfig(c); e != nil {
		panic(e)
	}
	stream := &Instance{
		conf:              c,
		status:            true,
		peernodes:         make([]*peernode, c.GroupNum),
		peerPool:          &sync.Pool{},
		websocketPeerPool: &sync.Pool{},
	}
	for i := range stream.peernodes {
		stream.peernodes[i] = &peernode{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peernodes[i])
	}
	return stream
}

func (this *Instance) heart(node *peernode) {
	tker := time.NewTicker(time.Duration(this.conf.HeartprobeInterval) * time.Millisecond)
	for {
		<-tker.C
		now := uint64(time.Now().UnixNano())
		node.RLock()
		for _, p := range node.peers {
			if !p.status {
				continue
			}
			templastactive := p.lastactive
			if now >= templastactive && now-templastactive > this.conf.HeartbeatTimeout*1000*1000 {
				//heartbeat timeout
				fmt.Printf("[Stream.%s.heart] timeout %s:%s addr:%s\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
				p.closeconn()
				p.status = false
			} else {
				var data []byte
				msg := &heartMsg{
					uniqueid:  p.starttime,
					sender:    p.getselfname(),
					timestamp: uint64(time.Now().UnixNano()),
				}
				switch p.protocoltype {
				case TCP:
					fallthrough
				case UNIXSOCKET:
					data = makeHeartMsg(msg, true)
				case WEBSOCKET:
					data = makeHeartMsg(msg, false)
				}
				select {
				case p.heartbeatbuffer <- data:
				default:
					fmt.Printf("[Stream.%s.heart] send heart msg to %s:%s addr:%s failed:heart buffer is full\n",
						p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
				}
			}
		}
		node.RUnlock()
	}
}
func (this *Instance) getindex(peername string) uint {
	result := uint(0)
	for _, v := range md5.Sum(str2byte(peername)) {
		result += uint(v)
	}
	return result % this.conf.GroupNum
}
