package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
)

type protocol int

const (
	TCP protocol = iota + 1
	UNIX
	WS
)

func (p protocol) protoname() string {
	switch p {
	case TCP:
		return "tcp"
	case UNIX:
		return "unix"
	case WS:
		return "ws"
	default:
		return ""
	}
}

type peertype int

const (
	CLIENT peertype = iota + 1
	SERVER
)

func (t peertype) typename() string {
	switch t {
	case CLIENT:
		return "client"
	case SERVER:
		return "server"
	default:
		return ""
	}
}

var (
	ERRCONNCLOSED  = errors.New("connection is closed")
	ERRMSGLENGTH   = errors.New("message length error")
	ERRSENDBUFFULL = errors.New("send buffer full")
)

type peernode struct {
	sync.RWMutex
	peers map[string]*Peer
}

type Peer struct {
	parentnode      *peernode
	clientname      string
	servername      string
	peertype        peertype
	protocol        protocol
	starttime       uint64
	closeread       bool
	closewrite      bool
	status          uint32 //0--(closed),1--(connected),2--(closing)
	maxmsglen       uint
	reader          *bufio.Reader
	writerbuffer    chan *bufpool.Buffer
	heartbeatbuffer chan *bufpool.Buffer
	conn            unsafe.Pointer
	fd              uint64
	lastactive      uint64         //unixnano timestamp
	recvidlestart   uint64         //unixnano timestamp
	sendidlestart   uint64         //unixnano timestamp
	data            unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

func (p *Peer) getUniqueName() string {
	var name, addr string
	switch p.peertype {
	case CLIENT:
		name = p.clientname
	case SERVER:
		name = p.servername
	}
	switch p.protocol {
	case WS:
		fallthrough
	case TCP:
		addr = (*net.TCPConn)(p.conn).RemoteAddr().String()
	case UNIX:
		addr = "fd:" + strconv.FormatUint(p.fd, 10)
	}
	if name == "" {
		return addr
	}
	return name + ":" + addr
}

//closeconn close the under layer socket
func (p *Peer) closeconn() {
	if p.conn != nil {
		switch p.protocol {
		case WS:
			fallthrough
		case TCP:
			(*net.TCPConn)(p.conn).Close()
		case UNIX:
			(*net.UnixConn)(p.conn).Close()
		}
	}
}

//closeRead just stop read
//write goruntine can still write data
func (p *Peer) closeRead() {
	p.writerbuffer <- makeCloseReadMsg(p.starttime, true)
}

//closeWrite just stop write
//read goruntine can still read data
func (p *Peer) closeWrite() {
	p.writerbuffer <- makeCloseWriteMsg(p.starttime, true)
}

func (p *Peer) setbuffer(readnum, writenum int) {
	switch p.protocol {
	case WS:
		fallthrough
	case TCP:
		(*net.TCPConn)(p.conn).SetReadBuffer(readnum)
		(*net.TCPConn)(p.conn).SetWriteBuffer(writenum)
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadBuffer(readnum)
		(*net.UnixConn)(p.conn).SetWriteBuffer(writenum)
	}
}

func (p *Peer) readMessage() (*bufpool.Buffer, error) {
	buf := bufpool.GetBuffer()
	buf.Grow(4)
	if _, e := io.ReadFull(p.reader, buf.Bytes()); e != nil {
		bufpool.PutBuffer(buf)
		return nil, e
	}
	num := int(binary.BigEndian.Uint32(buf.Bytes()))
	if num > int(p.maxmsglen) || num < 0 {
		bufpool.PutBuffer(buf)
		return nil, ERRMSGLENGTH
	} else if num == 0 {
		bufpool.PutBuffer(buf)
		return nil, nil
	}
	buf.Grow(num)
	if _, e := io.ReadFull(p.reader, buf.Bytes()); e != nil {
		bufpool.PutBuffer(buf)
		return nil, e
	}
	return buf, nil
}
func (p *Peer) SendMessage(userdata []byte, starttime uint64, block bool) error {
	if len(userdata) == 0 {
		return nil
	}
	if len(userdata) > int(p.maxmsglen) {
		return ERRMSGLENGTH
	}
	if p.closeread || p.status == 0 || p.status == 2 || p.starttime != starttime {
		//starttime for aba check
		return ERRCONNCLOSED
	}
	data := makeUserMsg(userdata, starttime, true)
	//here has a little data race,but never mind,peer will drop the race data
	if block {
		p.writerbuffer <- data
	} else {
		select {
		case p.writerbuffer <- data:
		default:
			return ERRSENDBUFFULL
		}
	}
	return nil
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) Close() {
	old := p.status
	if old == 1 && atomic.CompareAndSwapUint32(&p.status, old, 2) {
		p.closeRead()
	}
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) CloseRead() {
	p.closeRead()
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) CloseWrite() {
	p.closeWrite()
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) GetData() unsafe.Pointer {
	return p.data
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) SetData(data unsafe.Pointer) {
	p.data = data
}
