package stream

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
)

var (
	ErrConnClosed = errors.New("connection closed")
	ErrMsgLarge   = errors.New("message too large")
	ErrMsgEmpty   = errors.New("message empty")
	ErrMsgUnknown = errors.New("message type unknown")
)

type Peer struct {
	peergroup      *group
	peeruniquename string
	status         int32 //1 - working,0 - closed
	selfmaxmsglen  uint32
	peermaxmsglen  uint32
	pingponger     chan *bufpool.Buffer
	dispatcher     chan *struct{}
	tls            bool
	conn           net.Conn
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
		pingponger:    make(chan *bufpool.Buffer, 3),
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
		p.conn.Close()
		return
	}
	if now-atomic.LoadInt64(&p.sendidlestart) > int64(sendidle) {
		//send idle timeout
		log.Error(nil, "[Stream.checkheart] send idle timeout:", p.peeruniquename)
		p.conn.Close()
		return
	}
	if recvidle != 0 && now-atomic.LoadInt64(&p.recvidlestart) > int64(recvidle) {
		//recv idle timeout
		log.Error(nil, "[Stream.checkheart] recv idle timeout:", p.peeruniquename)
		p.conn.Close()
		return
	}
	pingdata := make([]byte, 8)
	binary.BigEndian.PutUint64(pingdata, uint64(now))
	//send heart beat data
	select {
	case p.pingponger <- makePingMsg(pingdata):
		go p.SendMessage(context.Background(), nil)
	default:
	}
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
func (p *Peer) SendMessage(ctx context.Context, userdata []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if e := p.getDispatcher(ctx); e != nil {
		return e
	}
	defer p.putDispatcher()
	var data *bufpool.Buffer
	var finish bool
	for {
		select {
		case data = <-p.pingponger:
		case <-ctx.Done():
			return ctx.Err()
		default:
			if len(userdata) == 0 {
				return nil
			}
			if len(userdata) > int(p.peermaxmsglen) {
				return ErrMsgLarge
			}
			data = makeUserMsg(userdata)
			finish = true
		}
		if _, e := p.conn.Write(data.Bytes()); e != nil {
			log.Error(nil, "[Stream.SendMessage] to:", p.peeruniquename, "error:", e)
			p.conn.Close()
			bufpool.PutBuffer(data)
			return ErrConnClosed
		}
		atomic.StoreInt64(&p.sendidlestart, time.Now().UnixNano())
		bufpool.PutBuffer(data)
		if finish {
			return nil
		}
	}
}

func (p *Peer) Close() {
	atomic.StoreInt32(&p.status, 0)
	p.conn.Close()
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
	return p.conn.RemoteAddr().String()
}
func (p *Peer) GetData() unsafe.Pointer {
	return p.data
}
func (p *Peer) SetData(data unsafe.Pointer) {
	p.data = data
}
