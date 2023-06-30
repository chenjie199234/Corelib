package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/ws"
)

var (
	ErrConnClosed = errors.New("connection closed")
	ErrMsgLarge   = errors.New("message too large")
)

const (
	_PEER_SERVER = 1
	_PEER_CLIENT = 2
)

type Peer struct {
	selfMaxMsgLen uint32
	peerMaxMsgLen uint32
	peergroup     *group
	status        int32 //1 - working,0 - closed
	dispatcher    chan *struct{}
	cr            *bufio.Reader
	c             net.Conn
	rawaddr       string //only useful when peertype is _PEER_SERVER
	peertype      int
	header        http.Header    //if this is not nil,means this is a websocket peer
	lastactive    int64          //unixnano timestamp
	recvidlestart int64          //unixnano timestamp
	sendidlestart int64          //unixnano timestamp
	netlag        int64          //unixnano
	data          unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

func newPeer(selfMaxMsgLen uint32, peertype int, rawaddr string) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{
		rawaddr:       rawaddr,
		peertype:      peertype,
		selfMaxMsgLen: selfMaxMsgLen,
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
	go func() {
		tmp := pool.GetBuffer()
		defer pool.PutBuffer(tmp)
		tmp.Resize(8)
		binary.BigEndian.PutUint64(tmp.Bytes(), uint64(now))
		if e := ws.WritePing(p.c, tmp.Bytes(), false); e != nil {
			log.Error(nil, "[Stream.checkheart] write ping to:", p.c.RemoteAddr().String(), e)
			p.c.Close()
			return
		}
		p.sendidlestart = now
	}()
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

// SendMessage will return ErrMsgLarge/ErrConnClosed/context.Canceled/context.DeadlineExceeded
func (p *Peer) SendMessage(ctx context.Context, userdata []byte, bs BeforeSend, as AfterSend) error {
	if len(userdata) == 0 {
		return nil
	}
	if uint64(len(userdata)) > uint64(p.peerMaxMsgLen) {
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
	first := true
	for len(userdata) > 0 {
		var data []byte
		if len(userdata) > maxPieceLen {
			data = userdata[:maxPieceLen]
			userdata = userdata[maxPieceLen:]
		} else {
			data = userdata
			userdata = nil
		}
		if e := ws.WriteMsg(p.c, data, userdata == nil, first, false); e != nil {
			log.Error(ctx, "[Stream.SendMessage] write to:", p.c.RemoteAddr().String(), e)
			p.c.Close()
			if as != nil {
				as(p, e)
			}
			return ErrConnClosed
		}
		p.sendidlestart = time.Now().UnixNano()
		first = false
	}
	if as != nil {
		as(p, nil)
	}
	return nil
}

func (p *Peer) Close() {
	atomic.StoreInt32(&p.status, 0)
	p.c.Close()
}

func (p *Peer) GetLocalPort() string {
	laddr := p.c.LocalAddr().String()
	return laddr[strings.LastIndex(laddr, ":")+1:]
}

func (p *Peer) GetNetlag() int64 {
	return atomic.LoadInt64(&p.netlag)
}

// only useful when peertype is _PEER_SERVER
func (p *Peer) GetRawAddr() string {
	return p.rawaddr
}

// get the direct peer's addr(maybe a proxy)
func (p *Peer) GetRemoteAddr() string {
	return p.c.RemoteAddr().String()
}

// this may be different with the RemoteAddr only when this is a websocket peer
func (p *Peer) GetRealPeerIP() string {
	var ip string
	if p.header != nil {
		if tmp := strings.TrimSpace(p.header.Get("X-Forwarded-For")); tmp != "" {
			ip = strings.TrimSpace(strings.Split(tmp, ",")[0])
		}
		if ip == "" {
			ip = strings.TrimSpace(p.header.Get("X-Real-Ip"))
		}
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(p.GetRemoteAddr())
	}
	return ip
}

// if this is not nil,means this is a websocket connection
func (p *Peer) GetHeader() http.Header {
	return p.header
}

func (p *Peer) GetPeerMaxMsgLen() uint32 {
	return p.peerMaxMsgLen
}

func (p *Peer) GetData() unsafe.Pointer {
	return atomic.LoadPointer(&p.data)
}

func (p *Peer) SetData(data unsafe.Pointer) {
	atomic.StorePointer(&p.data, data)
}
