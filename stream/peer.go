package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
)

var (
	ErrConnClosed  = errors.New("connection closed")
	ErrMsgTooLarge = errors.New("message too large")
)

type Peer struct {
	selfmaxmsglen  uint32
	peermaxmsglen  uint32
	peergroup      *group
	peeruniquename string
	status         int32 //1 - working,0 - closed
	dispatcher     chan *struct{}
	cr             *bufio.Reader
	c              net.Conn
	lastactive     int64          //unixnano timestamp
	recvidlestart  int64          //unixnano timestamp
	sendidlestart  int64          //unixnano timestamp
	netlag         int64          //unixnano
	data           unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

func newPeer(selfmaxmsglen uint32) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{
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
		log.Error(nil, "[Stream.checkheart] heart timeout:", p.peeruniquename)
		p.c.Close()
		return
	}
	if now-atomic.LoadInt64(&p.sendidlestart) > int64(sendidle) {
		//send idle timeout
		log.Error(nil, "[Stream.checkheart] send idle timeout:", p.peeruniquename)
		p.c.Close()
		return
	}
	if recvidle != 0 && now-atomic.LoadInt64(&p.recvidlestart) > int64(recvidle) {
		//recv idle timeout
		log.Error(nil, "[Stream.checkheart] recv idle timeout:", p.peeruniquename)
		p.c.Close()
		return
	}
	pingdata := make([]byte, 8)
	binary.BigEndian.PutUint64(pingdata, uint64(now))
	//send heart beat data
	go p.sendPing(pingdata)
}

func (p *Peer) readMessage(total *pool.Buffer, tmp *pool.Buffer) (fin bool, mtype int, e error) {
	tmp.Resize(3)
	if _, e = io.ReadFull(p.cr, tmp.Bytes()); e != nil {
		return
	}
	num := binary.BigEndian.Uint16(tmp.Bytes()[:2])
	if num == 0 {
		e = ErrMsgEmpty
		return
	}
	if fin, mtype, e = decodeHeader(tmp.Bytes()[2]); e != nil {
		return
	}
	if mtype != _USER || total == nil {
		tmp.Resize(uint32(num) - 1)
		_, e = io.ReadFull(p.cr, tmp.Bytes())
	} else if oldlen := uint32(total.Len()); oldlen+uint32(num)-1 > p.selfmaxmsglen {
		e = ErrMsgTooLarge
		return
	} else {
		total.Growth(oldlen + uint32(num) - 1)
		_, e = io.ReadFull(p.cr, total.Bytes()[oldlen:])
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
	if len(userdata) > int(p.peermaxmsglen) {
		return ErrMsgTooLarge
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
	for len(userdata) > 0 {
		var data *pool.Buffer
		if len(userdata) > maxPieceLen {
			data = makeUserMsg(userdata[:maxPieceLen], false)
			userdata = userdata[maxPieceLen:]
		} else {
			data = makeUserMsg(userdata, true)
			userdata = nil
		}
		if _, e := p.c.Write(data.Bytes()); e != nil {
			log.Error(ctx, "[Stream.SendMessage] to:", p.peeruniquename, "error:", e)
			p.c.Close()
			pool.PutBuffer(data)
			if as != nil {
				as(p, e)
			}
			return ErrConnClosed
		}
		atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
		pool.PutBuffer(data)
	}
	if as != nil {
		as(p, nil)
	}
	return nil
}
func (p *Peer) sendPing(pingdata []byte) error {
	data := makePingMsg(pingdata)
	if _, e := p.c.Write(data.Bytes()); e != nil {
		log.Error(nil, "[Stream.sendPing] to:", p.peeruniquename, "error:", e)
		p.c.Close()
		pool.PutBuffer(data)
		return ErrConnClosed
	}
	atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
	pool.PutBuffer(data)
	return nil
}
func (p *Peer) sendPong(pongdata *pool.Buffer) error {
	data := makePongMsg(pongdata.Bytes())
	if _, e := p.c.Write(data.Bytes()); e != nil {
		log.Error(nil, "[Stream.sendPong] to:", p.peeruniquename, "error:", e)
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

//peername
func (p *Peer) GetPeerName() string {
	return p.peeruniquename[:strings.Index(p.peeruniquename, ":")]
}

//peername:ip:port
func (p *Peer) GetPeerUniqueName() string {
	return p.peeruniquename
}
func (p *Peer) GetPeerNetlag() int64 {
	return atomic.LoadInt64(&p.netlag)
}
func (p *Peer) GetRemoteAddr() string {
	return p.c.RemoteAddr().String()
}
func (p *Peer) GetPeerMaxMsgLen() uint32 {
	return p.peermaxmsglen
}
func (p *Peer) GetData() unsafe.Pointer {
	return p.data
}
func (p *Peer) SetData(data unsafe.Pointer) {
	p.data = data
}
