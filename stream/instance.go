package stream

import (
	"crypto/md5"
	"errors"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/buffer"
)

//Warning!!Don't write block logic in these callback,live for{}
type HandleOnlineFunc func(p *Peer, uniqueid int64)
type HandleVerifyFunc func(selfVerifyData, peerVerifyData []byte, peername string) bool
type HandleUserdataFunc func(p *Peer, uniqueid int64, data []byte)
type HandleOfflineFunc func(p *Peer, uniqueid int64)

const (
	PEERTCP = iota + 1
	PEERWEBSOCKET
	PEERUNIXSOCKET
)

var (
	ERRCONNCLOSED   = errors.New("connection is closed")
	ERRSERVERCLOSED = errors.New("server is closed")
	ERRFULL         = errors.New("write buffer is full")
)

type peernode struct {
	sync.RWMutex
	peers map[string]*Peer
}
type Peer struct {
	parentnode    *peernode
	name          string
	peertype      int
	starttime     int64
	readbuffer    *buffer.Buffer
	tempbuffer    []byte
	tempbuffernum int
	writerbuffer  chan []byte
	conn          unsafe.Pointer
	lastactive    int64
	status        bool //true-working,false-closing
}

func (p *Peer) Close() {
	p.parentnode.RLock()
	p.closeconn()
	p.status = false
	p.parentnode.RUnlock()
}
func (p *Peer) closeconn() {
	if p.conn != nil {
		switch p.peertype {
		case PEERTCP:
			(*net.TCPConn)(p.conn).Close()
		case PEERWEBSOCKET:
		case PEERUNIXSOCKET:
		}
	}
}
func (p *Peer) setbuffer(num int) {
	switch p.peertype {
	case PEERTCP:
		(*net.TCPConn)(p.conn).SetNoDelay(true)
		(*net.TCPConn)(p.conn).SetReadBuffer(num)
		(*net.TCPConn)(p.conn).SetWriteBuffer(num)
	case PEERWEBSOCKET:
	case PEERUNIXSOCKET:
	}
}

type Instance struct {
	conf         *Config
	verifyfunc   HandleVerifyFunc
	onlinefunc   HandleOnlineFunc
	userdatafunc HandleUserdataFunc
	offlinefunc  HandleOfflineFunc
	status       bool //true-working,false-closing
	peerPool     *sync.Pool
	sync.RWMutex
	peernodes []*peernode
}

func (this *Instance) getpeer() *Peer {
	return this.peerPool.Get().(*Peer)
}
func (this *Instance) putpeer(p *Peer) {
	p.name = ""
	p.peertype = 0
	p.starttime = 0
	p.readbuffer.Reset()
	p.tempbuffernum = 0
	if len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	p.closeconn()
	p.status = false
	this.peerPool.Put(p)
}
func NewInstance(c *Config, verify HandleVerifyFunc, online HandleOnlineFunc, userdata HandleUserdataFunc, offline HandleOfflineFunc) *Instance {
	//online and offline can be nill
	//verify and userdata can't be nill
	if verify == nil {
		return nil
	}
	if userdata == nil {
		return nil
	}
	if c.MinReadBufferLen == 0 {
		c.MinReadBufferLen = 1024
	}
	if c.MaxReadBufferLen == 0 {
		c.MaxReadBufferLen = 40960
	}
	if c.MaxReadBufferLen < c.MinReadBufferLen {
		c.MaxReadBufferLen = c.MinReadBufferLen
	}
	if c.MaxWriteBufferNum == 0 {
		c.MaxWriteBufferNum = 256
	}
	if c.splitnum == 0 {
		c.splitnum = 1
	}
	stream := &Instance{
		conf:         c,
		verifyfunc:   verify,
		onlinefunc:   online,
		userdatafunc: userdata,
		offlinefunc:  offline,
		status:       true,
		peerPool: &sync.Pool{
			New: func() interface{} {
				return &Peer{
					name:          "",
					peertype:      0,
					starttime:     0,
					readbuffer:    buffer.NewBuf(c.MinReadBufferLen, c.MaxReadBufferLen),
					tempbuffer:    make([]byte, c.MinReadBufferLen),
					tempbuffernum: 0,
					writerbuffer:  make(chan []byte, c.MaxWriteBufferNum),
					conn:          nil,
					lastactive:    0,
					status:        false,
				}
			},
		},
		peernodes: make([]*peernode, c.splitnum),
	}
	for i := range stream.peernodes {
		stream.peernodes[i] = &peernode{
			peers: make(map[string]*Peer),
		}
		go stream.heart(stream.peernodes[i])
	}
	return stream
}
func (this *Instance) SendMessage(p *Peer, userdata []byte, uniqueid int64) error {
	if !this.status {
		return ERRSERVERCLOSED
	}
	if !p.status || p.starttime != uniqueid {
		//status for close check
		//uniqueid for ABA check
		return ERRCONNCLOSED
	}
	//here has little data race,but the message package will be dropped by peer,because of different uniqueid
	select {
	case p.writerbuffer <- makeUserMsg(this.conf.SelfName, userdata, uniqueid):
		return nil
	default:
		return ERRFULL
	}
}
func (this *Instance) heart(node *peernode) {
	tker := time.NewTicker(time.Duration(this.conf.HeartInterval/2) * time.Millisecond)
	for {
		<-tker.C
		now := time.Now().UnixNano()
		node.RLock()
		for _, p := range node.peers {
			if now-p.lastactive > this.conf.HeartInterval*1000*1000 {
				//heartbeat timeout
				if p.conn != nil {
					//peer.conn.Close()
				}
				p.status = false
			} else {
				p.writerbuffer <- makeHeartMsg(this.conf.SelfName, time.Now().UnixNano(), p.starttime)
			}
		}
		node.RUnlock()
	}
}
func (this *Instance) getindex(peername string) int {
	result := 0
	for _, v := range md5.Sum(str2byte(peername)) {
		result += int(v)
	}
	return result % this.conf.splitnum
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
