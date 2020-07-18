package stream

import (
	"fmt"
	"net"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/buffer"
	"github.com/gorilla/websocket"
)

const (
	TCP = iota + 1
	WEBSOCKET
	UNIXSOCKET
)

var PROTOCOLNAME = []string{TCP: "TCP", WEBSOCKET: "WEB", UNIXSOCKET: "UNIX"}

const (
	CLIENT = iota + 1
	SERVER
)

var PEERTYPENAME = []string{CLIENT: "client", SERVER: "server"}

var (
	ERRCONNCLOSED = fmt.Errorf("connection is closed")
	ERRFULL       = fmt.Errorf("write buffer is full")
)

type peernode struct {
	sync.RWMutex
	peers map[string]*Peer
}
type Peer struct {
	parentnode      *peernode
	clientname      string
	servername      string
	peertype        int
	protocoltype    int
	starttime       int64
	readbuffer      *buffer.Buffer
	tempbuffer      []byte
	tempbuffernum   int
	writerbuffer    chan []byte
	heartbeatbuffer chan []byte
	conn            unsafe.Pointer
	lastactive      int64   //unixnano timestamp
	netlag          []int64 //unixnano timeoffset
	netlagindex     int
	status          bool //true-working,false-closing
}

func (p *Peer) getprotocolname() string {
	return PROTOCOLNAME[p.protocoltype]
}
func (p *Peer) getpeertypename() string {
	return PEERTYPENAME[p.peertype]
}
func (p *Peer) getpeername() string {
	switch p.peertype {
	case CLIENT:
		return p.clientname
	case SERVER:
		return p.servername
	}
	return ""
}
func (p *Peer) getselfname() string {
	switch p.peertype {
	case CLIENT:
		return p.servername
	case SERVER:
		return p.clientname
	}
	return ""
}
func (p *Peer) getpeeraddr() string {
	switch p.protocoltype {
	case TCP:
		return (*net.TCPConn)(p.conn).RemoteAddr().String()
	case UNIXSOCKET:
		return (*net.UnixConn)(p.conn).RemoteAddr().String()
	case WEBSOCKET:
		return (*websocket.Conn)(p.conn).RemoteAddr().String()
	}
	return ""
}
func (p *Peer) getselfaddr() string {
	switch p.protocoltype {
	case TCP:
		return (*net.TCPConn)(p.conn).LocalAddr().String()
	case UNIXSOCKET:
		return (*net.UnixConn)(p.conn).LocalAddr().String()
	case WEBSOCKET:
		return (*websocket.Conn)(p.conn).LocalAddr().String()
	}
	return ""
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
			(*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn).CloseRead()
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
			(*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn).CloseWrite()
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
		(*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn).SetReadBuffer(readnum)
		(*websocket.Conn)(p.conn).UnderlyingConn().(*net.TCPConn).SetWriteBuffer(writenum)
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

//unit nanosecond
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

//unit nanosecond
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
