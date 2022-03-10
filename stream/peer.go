package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
)

var (
	ErrConnClosed = errors.New("connection closed")
)

const (
	_PEER_SERVER = 1
	_PEER_CLIENT = 2
)

type Peer struct {
	selfmaxmsglen uint32
	peermaxmsglen uint32
	peergroup     *group
	status        int32 //1 - working,0 - closed
	dispatcher    chan *struct{}
	cr            *bufio.Reader
	c             net.Conn
	ws            bool
	peertype      int
	realPeerIP    string
	lastactive    int64          //unixnano timestamp
	recvidlestart int64          //unixnano timestamp
	sendidlestart int64          //unixnano timestamp
	netlag        int64          //unixnano
	data          unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

func newPeer(selfmaxmsglen uint32, peertype int) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{
		peertype:      peertype,
		selfmaxmsglen: selfmaxmsglen,
		dispatcher:    make(chan *struct{}, 1),
		Context:       ctx,
		CancelFunc:    cancel,
	}
	p.dispatcher <- nil
	return p
}

func (p *Peer) checkheart(heart, sendidle, recvidle time.Duration, nowtime *time.Time) {
	if atomic.LoadInt32(&p.status) != 1 {
		return
	}
	now := nowtime.UnixNano()
	if now-atomic.LoadInt64(&p.lastactive) > int64(heart) {
		//heartbeat timeout
		log.Error(nil, "[Stream.checkheart] heart timeout:", p.c.RemoteAddr().String())
		p.c.Close()
		return
	}
	if now-atomic.LoadInt64(&p.sendidlestart) > int64(sendidle) {
		//send idle timeout
		log.Error(nil, "[Stream.checkheart] send idle timeout:", p.c.RemoteAddr().String())
		p.c.Close()
		return
	}
	if recvidle != 0 && now-atomic.LoadInt64(&p.recvidlestart) > int64(recvidle) {
		//recv idle timeout
		log.Error(nil, "[Stream.checkheart] recv idle timeout:", p.c.RemoteAddr().String())
		p.c.Close()
		return
	}
	//send heart beat data
	go p.sendPing(now)
}

func (p *Peer) readMessage(total *pool.Buffer, tmp *pool.Buffer) (fin bool, opcode int, e error) {
	b, e := p.cr.ReadByte()
	if e != nil {
		return
	}
	if fin, _, _, _, opcode, e = decodeFirst(b); e != nil {
		return
	}
	b, e = p.cr.ReadByte()
	if e != nil {
		return
	}
	mask, payload, e := decodeSecond(b, opcode, p.peertype == _PEER_CLIENT && p.ws)
	if e != nil {
		return
	}
	var length uint32
	switch payload {
	case 127:
		tmp.Resize(8)
		if _, e = io.ReadFull(p.cr, tmp.Bytes()); e != nil {
			return
		}
		tmplen := binary.BigEndian.Uint64(tmp.Bytes())
		if tmplen > uint64(p.selfmaxmsglen) {
			e = ErrMsgLarge
			return
		}
		length = uint32(tmplen)
	case 126:
		tmp.Resize(2)
		if _, e = io.ReadFull(p.cr, tmp.Bytes()); e != nil {
			return
		}
		length = uint32(binary.BigEndian.Uint16(tmp.Bytes()))
	default:
		length = payload
	}
	var maskkey *pool.Buffer
	if mask {
		maskkey = pool.GetBuffer()
		defer pool.PutBuffer(maskkey)
		maskkey.Resize(4)
		if _, e = io.ReadFull(p.cr, maskkey.Bytes()); e != nil {
			return
		}
	}
	if length == 0 {
		return
	}
	if iscontrol(opcode) || total == nil {
		tmp.Resize(length)
		if _, e = io.ReadFull(p.cr, tmp.Bytes()); e != nil {
			return
		}
		if mask {
			domask(tmp.Bytes(), maskkey.Bytes())
		}
	} else if uint32(total.Len())+length > p.selfmaxmsglen {
		e = ErrMsgLarge
		return
	} else {
		total.Growth(uint32(total.Len()) + length)
		if _, e = io.ReadFull(p.cr, total.Bytes()[uint32(total.Len())-length:]); e != nil {
			return
		}
		if mask {
			domask(total.Bytes()[uint32(total.Len())-length:], maskkey.Bytes())
		}
	}
	return
}

func (p *Peer) getDispatcher(ctx context.Context) error {
	//first check
	if atomic.LoadInt32(&p.status) != 1 {
		return ErrConnClosed
	}
	select {
	case _, ok := <-p.dispatcher:
		if !ok {
			return ErrConnClosed
		} else if atomic.LoadInt32(&p.status) != 1 {
			//double check
			close(p.dispatcher)
			return ErrConnClosed
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *Peer) putDispatcher() {
	if atomic.LoadInt32(&p.status) == 1 {
		p.dispatcher <- nil
	} else {
		close(p.dispatcher)
	}
}

type BeforeSend func(*Peer)
type AfterSend func(*Peer, error)

func (p *Peer) SendMessage(ctx context.Context, userdata []byte, bs BeforeSend, as AfterSend) error {
	if len(userdata) == 0 {
		return nil
	}
	if uint64(len(userdata)) > uint64(p.peermaxmsglen) {
		return ErrMsgLarge
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if e := p.getDispatcher(ctx); e != nil {
		return e
	}
	defer p.putDispatcher()
	if bs != nil {
		bs(p)
	}
	piece := 0
	for len(userdata) > 0 {
		var data *pool.Buffer
		var e error
		if len(userdata) > maxPieceLen {
			//send to websocket server must mask data
			data = makeCommonMsg(userdata[:maxPieceLen], false, piece, p.peertype == _PEER_SERVER && p.ws)
			userdata = userdata[maxPieceLen:]
		} else {
			//send to websocket server must mask data
			data = makeCommonMsg(userdata, true, piece, p.peertype == _PEER_SERVER && p.ws)
			userdata = nil
		}
		if e != nil {
			return e
		}
		if _, e := p.c.Write(data.Bytes()); e != nil {
			log.Error(ctx, "[Stream.SendMessage] to:", p.c.RemoteAddr().String(), "error:", e)
			p.c.Close()
			pool.PutBuffer(data)
			if as != nil {
				as(p, e)
			}
			return ErrConnClosed
		}
		atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
		pool.PutBuffer(data)
		piece++
	}
	if as != nil {
		as(p, nil)
	}
	return nil
}
func (p *Peer) sendPing(nowUnixnano int64) error {
	//send to websocket server must mask data
	data := makePingMsg(nowUnixnano, p.peertype == _PEER_SERVER && p.ws)
	if _, e := p.c.Write(data.Bytes()); e != nil {
		log.Error(nil, "[Stream.sendPing] to:", p.c.RemoteAddr().String(), "error:", e)
		p.c.Close()
		pool.PutBuffer(data)
		return ErrConnClosed
	}
	atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
	pool.PutBuffer(data)
	return nil
}
func (p *Peer) sendPong(pongdata *pool.Buffer) error {
	//send to websocket server must mask data
	data := makePongMsg(pongdata.Bytes(), p.peertype == _PEER_SERVER && p.ws)
	if _, e := p.c.Write(data.Bytes()); e != nil {
		log.Error(nil, "[Stream.sendPong] to:", p.c.RemoteAddr().String(), "error:", e)
		p.c.Close()
		pool.PutBuffer(data)
		return ErrConnClosed
	}
	atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
	pool.PutBuffer(data)
	return nil
}
func (p *Peer) Close() {
	atomic.StoreInt32(&p.status, 0)
	p.c.Close()
}
func (p *Peer) GetPeerNetlag() int64 {
	return atomic.LoadInt64(&p.netlag)
}
func (p *Peer) GetRemoteAddr() string {
	return p.c.RemoteAddr().String()
}

//when the connection is based on websocket and there is a load balancer before it
//then the realPeerIP may different from the remoteaddr
func (p *Peer) GetRealPeerIp() string {
	return p.realPeerIP
}
func (p *Peer) GetPeerMaxMsgLen() uint32 {
	return p.peermaxmsglen
}
func (p *Peer) GetData() unsafe.Pointer {
	return atomic.LoadPointer(&p.data)
}
func (p *Peer) SetData(data unsafe.Pointer) {
	atomic.StorePointer(&p.data, data)
}
