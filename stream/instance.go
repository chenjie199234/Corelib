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
	"github.com/gorilla/websocket"
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
	peertype      int
	protocoltype  int
	starttime     int64
	readbuffer    *buffer.Buffer
	tempbuffer    []byte
	tempbuffernum int
	writerbuffer  chan []byte
	conn          unsafe.Pointer
	lastactive    int64   //unixnano timestamp
	netlag        []int64 //unixnano timeoffset
	netlagindex   int
	status        bool //true-working,false-closing
}

func (p *Peer) closeconn() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).Close()
		case UNIXSOCKET:
			(*net.UnixConn)(p.conn).Close()
		case WEBSOCKET:
			(*websocket.Conn)(p.conn).Close()
		}
	}
}
func (p *Peer) closeconnread() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).CloseRead()
		case UNIXSOCKET:
			(*net.UnixConn)(p.conn).CloseRead()
		case WEBSOCKET:
			if t, ok := (*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn); ok {
				if e := t.CloseRead(); e != nil {
					fmt.Printf("can't closeread tcpconn for websocket,error:%s\n", e)
				}
			} else {
				fmt.Println("can't change websocket conn to tcp conn")
			}
		}
	}
}
func (p *Peer) closeconnwrite() {
	if p.conn != nil {
		switch p.protocoltype {
		case TCP:
			(*net.TCPConn)(p.conn).CloseWrite()
		case UNIXSOCKET:
			(*net.UnixConn)(p.conn).CloseWrite()
		case WEBSOCKET:
			if t, ok := (*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn); ok {
				if e := t.CloseWrite(); e != nil {
					fmt.Printf("can't closewrite tcpconn for websocket,error:%s\n", e)
				}
			} else {
				fmt.Println("can't change webcosket conn to tcp conn")
			}
		}
	}
}
func (p *Peer) setbuffer(readnum, writenum int) {
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadBuffer(readnum)
		(*net.TCPConn)(p.conn).SetWriteBuffer(writenum)
	case UNIXSOCKET:
		(*net.UnixConn)(p.conn).SetReadBuffer(readnum)
		(*net.UnixConn)(p.conn).SetWriteBuffer(writenum)
	case WEBSOCKET:
		if t, ok := (*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn); ok {
			if e := t.SetReadBuffer(readnum); e != nil {
				fmt.Printf("can't set readbuffer tcpconn for websocket,error:%s\n", e)
			}
			if e := t.SetWriteBuffer(writenum); e != nil {
				fmt.Printf("can't set writebuffer tcpconn for websocket,error:%s\n", e)
			}
		} else {
			fmt.Println("can't change webcosket conn to tcp conn")
		}
	}
}

func (p *Peer) Close(uniqueid int64) {
	p.parentnode.RLock()
	if uniqueid != 0 && p.starttime == uniqueid && p.status {
		p.closeconn()
		p.status = false
	}
	p.parentnode.RUnlock()
}

//get the average netlag within the sample collect cycle
func (p *Peer) GetAverageNetLag() int64 {
	total := int64(0)
	count := int64(0)
	for _, v := range p.netlag {
		if v != 0 {
			total += v
			count++
		}
	}
	return total / count
}

//get the max netlag within the sample collect cycle
func (p *Peer) GetPeekNetLag() int64 {
	max := p.netlag[0]
	for _, v := range p.netlag {
		if max < v {
			max = v
		}
	}
	return max
}

func (p *Peer) SendMessage(userdata []byte, uniqueid int64) error {
	if !p.status || uniqueid == 0 || p.starttime != uniqueid {
		//status for close check
		//uniqueid for ABA check
		return ERRCONNCLOSED
	}
	//here has little data race,but the message package will be dropped by peer,because of different uniqueid
	var data []byte
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		data = makeUserMsg(userdata, uniqueid, true)
	case WEBSOCKET:
		data = makeUserMsg(userdata, uniqueid, false)
	}
	select {
	case p.writerbuffer <- data:
	default:
		return ERRFULL
	}
	return nil
}

func (this *Instance) SendMessage(peername string, userdata []byte) error {
	node := this.peernodes[this.getindex(peername)]
	node.RLock()
	p, ok := node.peers[peername]
	if !ok {
		node.RUnlock()
		return ERRCONNCLOSED
	}
	var data []byte
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		data = makeUserMsg(userdata, p.starttime, true)
	case WEBSOCKET:
		data = makeUserMsg(userdata, p.starttime, false)
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

func (this *Instance) GetAverageNetLag(peername string) (int64, error) {
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

func (this *Instance) GetPeekNetLag(peername string) (int64, error) {
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
	conf              *Config
	verifyfunc        HandleVerifyFunc
	onlinefunc        HandleOnlineFunc
	userdatafunc      HandleUserdataFunc
	offlinefunc       HandleOfflineFunc
	status            bool //true-working,false-closing
	peerPool          *sync.Pool
	websocketPeerPool *sync.Pool
	peernodes         []*peernode
}

func (this *Instance) getPeer() *Peer {
	return this.peerPool.Get().(*Peer)
}
func (this *Instance) putPeer(p *Peer) {
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	p.peertype = 0
	p.protocoltype = 0
	p.starttime = 0
	p.readbuffer.Reset()
	p.tempbuffernum = 0
	if len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	p.closeconn()
	p.lastactive = 0
	for i := range p.netlag {
		p.netlag[i] = 0
	}
	p.netlagindex = 0
	p.status = false
	this.peerPool.Put(p)
}
func (this *Instance) getWebsocketPeer() *Peer {
	return this.websocketPeerPool.Get().(*Peer)
}
func (this *Instance) putWebSocketPeer(p *Peer) {
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	p.peertype = 0
	p.protocoltype = 0
	p.starttime = 0
	p.readbuffer = nil
	p.tempbuffer = nil
	p.tempbuffernum = 0
	if len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	p.closeconn()
	p.lastactive = 0
	for i := range p.netlag {
		p.netlag[i] = 0
	}
	p.netlagindex = 0
	p.status = false
	this.websocketPeerPool.Put(p)
}
func (this *Instance) addPeer(p *Peer) bool {
	var node *peernode
	var ok bool
	switch p.peertype {
	case CLIENT:
		node = this.peernodes[this.getindex(p.clientname)]
	case SERVER:
		node = this.peernodes[this.getindex(p.servername)]
	}
	node.Lock()
	switch p.peertype {
	case CLIENT:
		_, ok = node.peers[p.clientname]
	case SERVER:
		_, ok = node.peers[p.servername]
	}
	if ok {
		p.closeconn()
		this.putPeer(p)
		node.Unlock()
		return false
	}
	p.parentnode = node
	switch p.peertype {
	case CLIENT:
		node.peers[p.clientname] = p
	case SERVER:
		node.peers[p.servername] = p
	}
	node.Unlock()
	return true
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

	checkConfig(c)

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
					peertype:      0,
					protocoltype:  0,
					starttime:     0,
					readbuffer:    buffer.NewBuf(c.AppMinReadBufferLen, c.AppMaxReadBufferLen),
					tempbuffer:    make([]byte, c.AppMinReadBufferLen),
					tempbuffernum: 0,
					writerbuffer:  make(chan []byte, c.AppWriteBufferNum),
					conn:          nil,
					lastactive:    0,
					netlag:        make([]int64, c.NetLagSampleNum),
					netlagindex:   0,
					status:        false,
				}
			},
		},
		websocketPeerPool: &sync.Pool{
			New: func() interface{} {
				return &Peer{
					clientname:    "",
					servername:    "",
					peertype:      0,
					protocoltype:  0,
					starttime:     0,
					readbuffer:    nil,
					tempbuffer:    nil,
					tempbuffernum: 0,
					writerbuffer:  make(chan []byte, c.AppWriteBufferNum),
					conn:          nil,
					netlag:        make([]int64, c.NetLagSampleNum),
					netlagindex:   0,
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
	tker := time.NewTicker(time.Duration(this.conf.HeartTimeout/3) * time.Millisecond)
	for {
		<-tker.C
		now := time.Now().UnixNano()
		node.RLock()
		for _, p := range node.peers {
			if !p.status {
				continue
			}
			if now-p.lastactive > this.conf.HeartTimeout*1000*1000 {
				//heartbeat timeout
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.heart] timeout client:%s addr:%s\n",
							p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.heart] timeout client:%s addr:%s\n",
							p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.heart] timeout server:%s addr:%s\n",
							p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.TCP.heart] timeout server:%s addr:%s\n",
							p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				}
				p.closeconnread()
				p.status = false
			} else {
				var data []byte
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fallthrough
					case UNIXSOCKET:
						data = makeHeartMsg(p.servername, time.Now().UnixNano(), p.starttime, true)
					case WEBSOCKET:
						data = makeHeartMsg(p.servername, time.Now().UnixNano(), p.starttime, false)
					}
					select {
					case p.writerbuffer <- data:
					default:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.heart] send heart msg to client:%s addr:%s failed:write buffer is full",
								p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.heart] send heart msg to client:%s addr:%s failed:write buffer is full",
								p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fallthrough
					case UNIXSOCKET:
						data = makeHeartMsg(p.clientname, time.Now().UnixNano(), p.starttime, true)
					case WEBSOCKET:
						data = makeHeartMsg(p.clientname, time.Now().UnixNano(), p.starttime, false)
					}
					select {
					case p.writerbuffer <- data:
					default:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.heart] send heart msg to server:%s addr:%s failed:write buffer is full",
								p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.heart] send heart msg to server:%s addr:%s failed:write buffer is full",
								p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					}
				}
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
