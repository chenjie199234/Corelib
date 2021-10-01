package stream

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
)

var (
	ErrConnClosed  = errors.New("connection closed")
	ErrMsgLarge    = errors.New("message too large")
	ErrMsgEmpty    = errors.New("message empty")
	ErrMsgUnknown  = errors.New("message type unknown")
	ErrSendBufFull = errors.New("send buffer full")
)

type peergroup struct {
	sync.RWMutex
	peers map[string]*Peer
}

type Peer struct {
	parentgroup    *peergroup
	peername       string
	sid            int64 //'<0'---(closing),'0'---(closed),'>0'---(connected)
	selfmaxmsglen  uint32
	peermaxmsglen  uint32
	writerbuffer   chan *bufpool.Buffer
	pingpongbuffer chan *bufpool.Buffer
	tls            bool
	rawconn        net.Conn
	conn           net.Conn
	lastactive     int64          //unixnano timestamp
	recvidlestart  int64          //unixnano timestamp
	sendidlestart  int64          //unixnano timestamp
	data           unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

func (p *Peer) getUniqueName() string {
	var addr string
	addr = p.conn.RemoteAddr().String()
	if p.peername == "" {
		return addr
	}
	return p.peername + ":" + addr
}

func (p *Peer) readMessage() (*bufpool.Buffer, error) {
	buf := bufpool.GetBuffer()
	buf.Resize(4)
	if _, e := io.ReadFull(p.conn, buf.Bytes()); e != nil {
		bufpool.PutBuffer(buf)
		return nil, e
	}
	num := binary.BigEndian.Uint32(buf.Bytes())
	if num > p.selfmaxmsglen || num < 0 {
		bufpool.PutBuffer(buf)
		return nil, ErrMsgLarge
	} else if num == 0 {
		bufpool.PutBuffer(buf)
		return nil, nil
	}
	buf.Resize(num)
	if _, e := io.ReadFull(p.conn, buf.Bytes()); e != nil {
		bufpool.PutBuffer(buf)
		return nil, e
	}
	return buf, nil
}

//if block is false,when the send buffer is full(depend on the MaxBufferedWriteMsgNum in config),error will return
//if block is true,when the send buffer is full(depend on the MaxBufferedWriteMsgNum in config),this call will block until send the data
func (p *Peer) SendMessage(userdata []byte, sid int64, block bool) error {
	if len(userdata) == 0 {
		return nil
	}
	if len(userdata) > int(p.peermaxmsglen) {
		return ErrMsgLarge
	}
	if p.sid != sid {
		//sid for aba check
		return ErrConnClosed
	}
	var data *bufpool.Buffer
	data = makeUserMsg(userdata, sid)
	//here has a little data race,but never mind,peer will drop the race data
	if block {
		p.writerbuffer <- data
	} else {
		select {
		case p.writerbuffer <- data:
		default:
			return ErrSendBufFull
		}
	}
	return nil
}

func (p *Peer) Close(sid int64) {
	if atomic.CompareAndSwapInt64(&p.sid, sid, -1) {
		//stop read new message
		p.rawconn.(*net.TCPConn).CloseRead()
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
