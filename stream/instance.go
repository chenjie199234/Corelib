package stream

import (
	"crypto/md5"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/buffer"
)

//Warning!!Don't write block logic in these callback,live for{}
type HandleOnlineFunc func(p *Peer, peername string, uniqueid int64)
type HandleVerifyFunc func(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool
type HandleUserdataFunc func(p *Peer, peername string, uniqueid int64, data []byte)
type HandleOfflineFunc func(p *Peer, peername string, uniqueid int64)

const (
	TCP = iota + 1
	WEBSOCKET
	UNIXSOCKET
)
const (
	CLIENT = iota + 1
	SERVER
)

var (
	ERRCONNCLOSED = errors.New("connection is closed")
	ERRFULL       = errors.New("write buffer is full")
)

type peernode struct {
	sync.RWMutex
	peers map[string]*Peer
}
type Peer struct {
	parentnode    *peernode
	clientname    string
	servername    string
	selftype      int
	protocoltype  int
	starttime     int64
	readbuffer    *buffer.Buffer
	tempbuffer    []byte
	tempbuffernum int
	writerbuffer  chan []byte
	conn          unsafe.Pointer
	lastactive    int64
	status        bool //true-working,false-closing
}

func (p *Peer) closeconn() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).Close()
		case WEBSOCKET:
		case UNIXSOCKET:
		}
	}
}
func (p *Peer) closeconnread() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).CloseRead()
		case WEBSOCKET:
		case UNIXSOCKET:
		}
	}
}
func (p *Peer) closeconnwrite() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).CloseWrite()
		case WEBSOCKET:
		case UNIXSOCKET:
		}
	}
}
func (p *Peer) setbuffer(num int) {
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetNoDelay(true)
		(*net.TCPConn)(p.conn).SetReadBuffer(num)
		(*net.TCPConn)(p.conn).SetWriteBuffer(num)
	case WEBSOCKET:
	case UNIXSOCKET:
	}
}
func (p *Peer) Close() {
	p.parentnode.RLock()
	p.closeconn()
	p.status = false
	p.parentnode.RUnlock()
}
func (p *Peer) SendMessage(userdata []byte, uniqueid int64) error {
	if !p.status || p.starttime != uniqueid {
		//status for close check
		//uniqueid for ABA check
		return ERRCONNCLOSED
	}
	//here has little data race,but the message package will be dropped by peer,because of different uniqueid
	var data []byte
	switch p.selftype {
	case CLIENT:
		data = makeUserMsg(p.clientname, userdata, uniqueid)
	case SERVER:
		data = makeUserMsg(p.servername, userdata, uniqueid)
	}
	select {
	case p.writerbuffer <- data:
		return nil
	default:
		return ERRFULL
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
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	p.selftype = 0
	p.protocoltype = 0
	p.starttime = 0
	p.readbuffer.Reset()
	p.tempbuffernum = 0
	if len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	p.closeconn()
	p.lastactive = 0
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
	if c.Splitnum == 0 {
		c.Splitnum = 1
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
					clientname:    "",
					servername:    "",
					selftype:      0,
					protocoltype:  0,
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
		peernodes: make([]*peernode, c.Splitnum),
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
	tker := time.NewTicker(time.Duration(this.conf.HeartInterval/3) * time.Millisecond)
	for {
		<-tker.C
		now := time.Now().UnixNano()
		node.RLock()
		for _, p := range node.peers {
			if !p.status {
				continue
			}
			if now-p.lastactive > this.conf.HeartInterval*1000*1000 {
				//heartbeat timeout
				switch p.selftype {
				case CLIENT:
					fmt.Printf("[Stream.heart] timeout server:%s\n", p.servername)
				case SERVER:
					fmt.Printf("[Stream.heart] timeout client:%s\n", p.clientname)
				}
				p.closeconnread()
				p.status = false
			} else {
				switch p.selftype {
				case CLIENT:
					p.writerbuffer <- makeHeartMsg(p.clientname, time.Now().UnixNano(), p.starttime)
				case SERVER:
					p.writerbuffer <- makeHeartMsg(p.servername, time.Now().UnixNano(), p.starttime)
				}
			}
		}
		//fmt.Println(len(node.peers))
		node.RUnlock()
	}
}
func (this *Instance) getindex(peername string) int {
	result := 0
	for _, v := range md5.Sum(str2byte(peername)) {
		result += int(v)
	}
	return result % this.conf.Splitnum
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
