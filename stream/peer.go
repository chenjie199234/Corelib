package stream

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
)

var (
	ErrConnClosed  = errors.New("connection closed")
	ErrMsgLarge    = errors.New("message too large")
	ErrMsgEmpty    = errors.New("message empty")
	ErrMsgUnknown  = errors.New("message type unknown")
	ErrSendBufFull = errors.New("send buffer full")
)

type Peer struct {
	peergroup      *group
	peername       string
	sid            int64 //'<0'---(closing),'0'---(closed),'>0'---(connected)
	selfmaxmsglen  uint32
	peermaxmsglen  uint32
	writerbuffer   chan *Msg
	pingpongbuffer chan *Msg
	tls            bool
	rawconn        net.Conn
	conn           net.Conn
	lastactive     int64          //unixnano timestamp
	recvidlestart  int64          //unixnano timestamp
	sendidlestart  int64          //unixnano timestamp
	data           unsafe.Pointer //user data
	lastunsend     []byte         //the last unsend message before the write closed
	context.Context
	context.CancelFunc
}

func (p *Peer) checkheart(heart, sendidle, recvidle time.Duration, nowtime *time.Time) {
	if p.sid <= 0 {
		return
	}
	now := nowtime.UnixNano()
	if now-p.lastactive > int64(heart) {
		//heartbeat timeout
		log.Error(nil, "[Stream.checkheart] heart timeout:", p.getUniqueName())
		p.conn.Close()
		return
	}
	if now-p.sendidlestart > int64(sendidle) {
		//send idle timeout
		log.Error(nil, "[Stream.checkheart] send idle timeout:", p.getUniqueName())
		p.conn.Close()
		return
	}
	if recvidle != 0 && now-p.recvidlestart > int64(recvidle) {
		//recv idle timeout
		log.Error(nil, "[Stream.checkheart] recv idle timeout:", p.getUniqueName())
		p.conn.Close()
		return
	}
	//send heart beat data
	select {
	case p.pingpongbuffer <- &Msg{mtype: PING}:
	default:
	}
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

//SendMessage is just write message into the write buffer,the write buffer length is depend on the config field:MaxBufferedWriteMsgNum
//if block is false,error will return when the send buffer is full,but if block is true,it will block until write the message into the write buffer
//if the ctx is cancelctx or timectx,it will be checked before actually write the message,but the error will not return.
func (p *Peer) SendMessage(ctx context.Context, userdata []byte, sid int64, block bool) error {
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
	data := &Msg{ctx: ctx, mtype: USER, sid: sid, data: userdata}
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
	//double check
	if p.sid >= 0 && p.sid != sid && !data.send {
		return ErrConnClosed
	}
	return nil
}

func (p *Peer) Close(sid int64) {
	if atomic.CompareAndSwapInt64(&p.sid, sid, -2) {
		//stop read new message
		p.rawconn.(*net.TCPConn).CloseRead()
		//wake up write goroutine
		select {
		case p.pingpongbuffer <- (*Msg)(nil):
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
