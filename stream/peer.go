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
	"github.com/gobwas/ws"
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

type peergroup struct {
	sync.RWMutex
	peers map[string]*Peer
}

type Peer struct {
	parentgroup    *peergroup
	clientname     string
	servername     string
	peertype       peertype
	protocol       protocol
	sid            int64 //'<0'---(closing),'0'---(closed),'>0'---(connected)
	maxmsglen      uint
	reader         *bufio.Reader
	writerbuffer   chan *bufpool.Buffer
	pingpongbuffer chan *bufpool.Buffer
	conn           unsafe.Pointer
	fd             uint64
	lastactive     int64          //unixnano timestamp
	recvidlestart  int64          //unixnano timestamp
	sendidlestart  int64          //unixnano timestamp
	data           unsafe.Pointer //user data
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
	if p.protocol == WS {
		header, e := ws.ReadHeader(p.reader)
		if e != nil {
			return nil, e
		}
		if header.Length > int64(p.maxmsglen) || header.Length < 0 {
			return nil, ERRMSGLENGTH
		}
		buf := bufpool.GetBuffer()
		var data []byte
		if header.OpCode == ws.OpPing {
			buf.Grow(1 + int(header.Length))
			buf.Bytes()[0] = byte(WSPING << 5)
			if header.Length == 0 {
				return buf, nil
			}
			data = buf.Bytes()[1:]
		} else if header.OpCode == ws.OpPong {
			buf.Grow(1 + int(header.Length))
			buf.Bytes()[0] = byte(WSPONG << 5)
			if header.Length == 0 {
				return buf, nil
			}
			data = buf.Bytes()[1:]
		} else if header.OpCode == ws.OpClose {
			return nil, io.EOF
		} else if header.Length == 0 {
			bufpool.PutBuffer(buf)
			return nil, nil
		} else {
			buf.Grow(int(header.Length))
			data = buf.Bytes()
		}
		if _, e := io.ReadFull(p.reader, data); e != nil {
			bufpool.PutBuffer(buf)
			return nil, e
		}
		if header.Masked {
			ws.Cipher(data, header.Mask, 0)
		}
		return buf, nil
	} else {
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
}

//if block is false,when the send buffer is full(depend on the MaxBufferedWriteMsgNum in config),error will return
//if block is true,when the send buffer is full(depend on the MaxBufferedWriteMsgNum in config),this call will block until send the data
func (p *Peer) SendMessage(userdata []byte, sid int64, block bool) error {
	if len(userdata) == 0 {
		return nil
	}
	if len(userdata) > int(p.maxmsglen) {
		return ERRMSGLENGTH
	}
	if p.sid != sid {
		//sid for aba check
		return ERRCONNCLOSED
	}
	var data *bufpool.Buffer
	if p.protocol == WS {
		data = makeUserMsg(userdata, sid, false)
	} else {
		data = makeUserMsg(userdata, sid, true)
	}
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
func (p *Peer) Close(sid int64) {
	if atomic.CompareAndSwapInt64(&p.sid, sid, -1) {
		//wake up write goroutine
		select {
		case p.pingpongbuffer <- (*bufpool.Buffer)(nil):
		default:
		}
	}
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
