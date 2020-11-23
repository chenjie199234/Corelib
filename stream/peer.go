package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"unsafe"

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
	ERRMSGLARGE   = fmt.Errorf("message too large")
)

type peernode struct {
	sync.RWMutex
	peers map[string]*Peer
}
type Peer struct {
	parentnode      *peernode
	clientname      *string
	servername      *string
	peertype        int
	protocoltype    int
	starttime       uint64
	status          uint32 //0--(closing),not 0--(connected)
	reader          *bufio.Reader
	writerbuffer    chan []byte
	heartbeatbuffer chan []byte
	conn            unsafe.Pointer
	lastactive      uint64 //unixnano timestamp
	recvidlestart   uint64 //unixnano timestamp
	sendidlestart   uint64 //unixnano timestamp
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
	p.clientname = nil
	p.servername = nil
	p.peertype = 0
	p.protocoltype = 0
	p.starttime = 0
	p.status = 0
	for len(p.writerbuffer) > 0 {
		<-p.writerbuffer
	}
	for len(p.heartbeatbuffer) > 0 {
		<-p.heartbeatbuffer
	}
	p.lastactive = 0
	p.recvidlestart = 0
	p.sendidlestart = 0
	p.data = nil
}
func (p *Peer) getprotocolname() string {
	return PROTOCOLNAME[p.protocoltype]
}
func (p *Peer) getpeertypename() string {
	return PEERTYPENAME[p.peertype]
}
func (p *Peer) getpeeruniquename() string {
	return p.getpeername() + ":" + p.getpeeraddr()
}
func (p *Peer) getpeername() string {
	switch p.peertype {
	case CLIENT:
		return *p.clientname
	case SERVER:
		return *p.servername
	}
	return ""
}
func (p *Peer) getselfuniquename() string {
	return p.getselfname() + ":" + p.getselfaddr()
}
func (p *Peer) getselfname() string {
	switch p.peertype {
	case CLIENT:
		return *p.servername
	case SERVER:
		return *p.clientname
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

func (p *Peer) readMessage(max int) ([]byte, error) {
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		temp := make([]byte, 4)
		num := 0
		for {
			n, e := p.reader.Read(temp[num:])
			if e != nil {
				return nil, e
			}
			num += n
			if num == 4 {
				break
			}
		}
		num = int(binary.BigEndian.Uint32(temp))
		if num > max || num < 0 {
			return nil, ERRMSGLARGE
		} else if num == 0 {
			return nil, nil
		}
		temp = make([]byte, num)
		for {
			n, e := p.reader.Read(temp[len(temp)-num:])
			if e != nil {
				return nil, e
			}
			num -= n
			if num == 0 {
				break
			}
		}
		return temp, nil
	case WEBSOCKET:
		_, data, e := (*websocket.Conn)(p.conn).ReadMessage()
		return data, e
	}
	return nil, nil
}
func (p *Peer) SendMessage(userdata []byte, starttime uint64) error {
	if p.status == 0 || p.starttime != starttime {
		//starttime for aba check
		return ERRCONNCLOSED
	}
	if len(userdata) == 0 {
		return nil
	}
	var data []byte
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		data = makeUserMsg(userdata, starttime, true)
	case WEBSOCKET:
		data = makeUserMsg(userdata, starttime, false)
	default:
		return ERRCONNCLOSED
	}
	//here has a little data race,but never mind,peer will drop the race data
	p.writerbuffer <- data
	return nil
}

//warning!has data race with HandleOfflineFunc
//this function can only be called before this peer's HandleOfflineFunc
func (p *Peer) Close() {
	p.closeconn()
}

//warning!has data race with HandleOfflineFunc
//this function can only be called before and in this peer's HandleOfflineFunc
func (p *Peer) GetData() unsafe.Pointer {
	return p.data
}

//warning!has data race with HandleOfflineFunc
//this function can only be called before this peer's HandleOfflineFunc
func (p *Peer) SetData(data unsafe.Pointer) {
	p.data = data
}
