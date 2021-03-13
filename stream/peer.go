package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
)

const (
	TCP = iota + 1
	UNIX
)

const (
	CLIENT = iota + 1
	SERVER
)

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
	peertype        uint
	protocoltype    uint
	starttime       uint64
	closeread       bool
	closewrite      bool
	status          uint32 //0--(closed),1--(connected),2--(closing)
	maxmsglen       uint
	reader          *bufio.Reader
	writerbuffer    chan *bufpool.Buffer
	heartbeatbuffer chan *bufpool.Buffer
	conn            unsafe.Pointer
	lastactive      uint64 //unixnano timestamp
	recvidlestart   uint64 //unixnano timestamp
	sendidlestart   uint64 //unixnano timestamp
	context.Context
	context.CancelFunc
	data unsafe.Pointer //user data
}

func (p *Peer) reset() {
	p.parentnode = nil
	p.clientname = ""
	p.servername = ""
	//p.peertype = 0
	//p.protocoltype = 0
	p.starttime = 0
	p.closeread = false
	p.closewrite = false
	//p.status = 0
	for len(p.writerbuffer) > 0 {
		if v := <-p.writerbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	for len(p.heartbeatbuffer) > 0 {
		if v := <-p.heartbeatbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	p.conn = nil
	p.lastactive = 0
	p.recvidlestart = 0
	p.sendidlestart = 0
	p.data = nil
}
func (p *Peer) getpeeruniquename() string {
	return p.getpeername() + ":" + p.getpeeraddr()
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
	return p.getselfname() + ":" + p.getselfaddr()
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
	case UNIX:
		c, e := (*net.UnixConn)(p.conn).SyscallConn()
		if e != nil {
			return ""
		}
		var fd uintptr
		c.Control(func(tempfd uintptr) {
			fd = tempfd
		})
		return (*net.UnixConn)(p.conn).RemoteAddr().String() + ":" + strconv.FormatUint(uint64(fd), 10)
	}
	return ""
}
func (p *Peer) getselfaddr() string {
	switch p.protocoltype {
	case TCP:
		return (*net.TCPConn)(p.conn).LocalAddr().String()
	case UNIX:
		c, e := (*net.UnixConn)(p.conn).SyscallConn()
		if e != nil {
			return ""
		}
		var fd uintptr
		c.Control(func(tempfd uintptr) {
			fd = tempfd
		})
		return (*net.UnixConn)(p.conn).RemoteAddr().String() + ":" + strconv.FormatUint(uint64(fd), 10)
	}
	return ""
}

//closeconn close the under layer socket
func (p *Peer) closeconn() {
	if p.conn != nil {
		switch p.protocoltype {
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
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadBuffer(readnum)
		(*net.TCPConn)(p.conn).SetWriteBuffer(writenum)
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadBuffer(readnum)
		(*net.UnixConn)(p.conn).SetWriteBuffer(writenum)
	}
}

func (p *Peer) readMessage(max uint) (*bufpool.Buffer, error) {
	buf := bufpool.GetBuffer()
	buf.Grow(4)
	num := 0
	for {
		n, e := p.reader.Read(buf.Bytes()[num:])
		if e != nil {
			bufpool.PutBuffer(buf)
			return nil, e
		}
		num += n
		if num == 4 {
			break
		}
	}
	num = int(binary.BigEndian.Uint32(buf.Bytes()))
	if num > int(max) || num < 0 {
		bufpool.PutBuffer(buf)
		return nil, ERRMSGLENGTH
	} else if num == 0 {
		bufpool.PutBuffer(buf)
		return nil, nil
	}
	buf.Grow(num)
	for {
		n, e := p.reader.Read(buf.Bytes()[buf.Len()-num:])
		if e != nil {
			bufpool.PutBuffer(buf)
			return nil, e
		}
		num -= n
		if num == 0 {
			break
		}
	}
	return buf, nil
}
func (p *Peer) SendMessage(userdata []byte, starttime uint64, block bool) error {
	if len(userdata) == 0 {
		return nil
	}
	if len(userdata) > int(p.maxmsglen) {
		switch p.protocoltype {
		case TCP:
			switch p.peertype {
			case CLIENT:
				log.Error("[Stream.TCP.SendMessage] send message to client:", p.getpeeruniquename(), "error:", ERRMSGLENGTH)
			case SERVER:
				log.Error("[Stream.TCP.SendMessage] send message to server:", p.getpeeruniquename(), "error:", ERRMSGLENGTH)
			}
		case UNIX:
			switch p.peertype {
			case CLIENT:
				log.Error("[Stream.UNIX.SendMessage] send message to client:", p.getpeeruniquename(), "error:", ERRMSGLENGTH)
			case SERVER:
				log.Error("[Stream.UNIX.SendMessage] send message to server:", p.getpeeruniquename(), "error:", ERRMSGLENGTH)
			}
		}
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
			switch p.protocoltype {
			case TCP:
				switch p.peertype {
				case CLIENT:
					log.Error("[Stream.TCP.SendMessage] send message to client:", p.getpeeruniquename(), "error:", ERRSENDBUFFULL)
				case SERVER:
					log.Error("[Stream.TCP.SendMessage] send message to server:", p.getpeeruniquename(), "error:", ERRSENDBUFFULL)
				}
			case UNIX:
				switch p.peertype {
				case CLIENT:
					log.Error("[Stream.UNIX.SendMessage] send message to client:", p.getpeeruniquename(), "error:", ERRSENDBUFFULL)
				case SERVER:
					log.Error("[Stream.UNIX.SendMessage] send message to server:", p.getpeeruniquename(), "error:", ERRSENDBUFFULL)
				}
			}
			return ERRSENDBUFFULL
		}
	}
	return nil
}

//warning!has data race
//this is only safe to use in callback func in sync mode
func (p *Peer) Close() {
	old := p.status
	if atomic.CompareAndSwapUint32(&p.status, old, 2) {
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
