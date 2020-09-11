package stream

import (
	"context"
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
	starttime       uint64 //0---(closed or closing),not 0---(start time)
	readbuffer      *buffer.Buffer
	tempbuffer      []byte
	tempbuffernum   int
	writerbuffer    chan []byte
	heartbeatbuffer chan []byte
	conn            unsafe.Pointer
	lastactive      uint64   //unixnano timestamp
	netlag          []uint64 //unixnano timeoffset
	netlagindex     int
	context.Context
	context.CancelFunc
	data unsafe.Pointer //user data
}

func (p *Peer) reset() {
	if p.CancelFunc != nil {
		p.CancelFunc()
	}
	p.closeconn()
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	p.peertype = 0
	p.protocoltype = 0
	p.starttime = 0
	if p.readbuffer != nil {
		p.readbuffer.Reset()
	}
	p.tempbuffernum = 0
	for len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	for len(p.heartbeatbuffer) > 0 {
		<-p.heartbeatbuffer
	}
	p.lastactive = 0
	for i := range p.netlag {
		p.netlag[i] = 0
	}
	p.netlagindex = 0
	p.data = nil
}
func (p *Peer) getprotocolname() string {
	return PROTOCOLNAME[p.protocoltype]
}
func (p *Peer) getpeertypename() string {
	return PEERTYPENAME[p.peertype]
}
func (p *Peer) getpeeruniquename() string {
	return p.getpeername() + "," + p.getpeeraddr()
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
func (p *Peer) getselfuniquename() string {
	return p.getselfname() + "," + p.getselfaddr()
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
		host, _, _ := net.SplitHostPort((*net.TCPConn)(p.conn).RemoteAddr().String())
		return host
	case UNIXSOCKET:
		return (*net.UnixConn)(p.conn).RemoteAddr().String()
	case WEBSOCKET:
		host, _, _ := net.SplitHostPort((*websocket.Conn)(p.conn).RemoteAddr().String())
		return host
	}
	return ""
}
func (p *Peer) getselfaddr() string {
	switch p.protocoltype {
	case TCP:
		host, _, _ := net.SplitHostPort((*net.TCPConn)(p.conn).LocalAddr().String())
		return host
	case UNIXSOCKET:
		return (*net.UnixConn)(p.conn).LocalAddr().String()
	case WEBSOCKET:
		host, _, _ := net.SplitHostPort((*websocket.Conn)(p.conn).LocalAddr().String())
		return host
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
	p.starttime = 0
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

func (p *Peer) GetData(uniqueid uint64) (unsafe.Pointer, error) {
	if uniqueid == p.starttime {
		return p.data, nil
	}
	return nil, ERRCONNCLOSED
}

func (p *Peer) SetData(data unsafe.Pointer, uniqueid uint64) error {
	if uniqueid == p.starttime {
		p.data = data
		return nil
	}
	return ERRCONNCLOSED
}

//unit nanosecond
func (p *Peer) GetAverageNetLag() uint64 {
	total := uint64(0)
	count := uint64(0)
	for _, v := range p.netlag {
		if v != 0 {
			total += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

//unit nanosecond
func (p *Peer) GetPeekNetLag() uint64 {
	max := p.netlag[0]
	for _, v := range p.netlag {
		if max < v {
			max = v
		}
	}
	return max
}

func (p *Peer) SendMessage(userdata []byte, uniqueid uint64) error {
	if uniqueid == 0 || p.starttime != uniqueid {
		//uniqueid for close check and ABA check
		return ERRCONNCLOSED
	}
	//here has little data race,but the message package will be dropped by peer,because of different uniqueid
	var data []byte
	msg := &userMsg{
		uniqueid: uniqueid,
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
		return ERRFULL
	}
	return nil
}

func (p *Peer) Close(uniqueid uint64) {
	p.parentnode.RLock()
	if uniqueid != 0 && p.starttime == uniqueid {
		p.closeconn()
	}
	p.parentnode.RUnlock()
}
