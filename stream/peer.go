package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/pool/bpool"
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
	uniqueid       string //if this is empty,the uniqueid will be setted with the peer's RemoteAddr(ip:port)
	blocknotice    chan *struct{}
	selfMaxMsgLen  atomic.Uint32
	peerMaxMsgLen  atomic.Uint32
	peergroup      *group
	status         atomic.Int32 //1 - working,0 - closed
	dispatcher     chan *struct{}
	cr             *bufio.Reader
	c              net.Conn
	rawconnectaddr string //only useful when peertype is _PEER_SERVER,this is the server's raw connect addr
	peertype       int
	header         http.Header    //if this is not nil,means this is a websocket peer
	lastactive     atomic.Int64   //unixnano timestamp
	recvidlestart  atomic.Int64   //unixnano timestamp
	sendidlestart  atomic.Int64   //unixnano timestamp
	netlag         atomic.Int64   //unixnano
	data           unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

// rawconnectaddr is only useful when peertype is _PEER_SERVER,this is the server's raw connect addr
func newPeer(selfMaxMsgLen uint32, peertype int, rawconnectaddr string) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{
		blocknotice:    make(chan *struct{}),
		rawconnectaddr: rawconnectaddr,
		peertype:       peertype,
		dispatcher:     make(chan *struct{}, 1),
		Context:        ctx,
		CancelFunc:     cancel,
	}
	p.selfMaxMsgLen.Store(selfMaxMsgLen)
	p.dispatcher <- nil
	return p
}

func (p *Peer) checkheart(heart, sendidle, recvidle time.Duration, nowtime *time.Time) {
	if p.status.Load() != 1 {
		return
	}
	now := nowtime.UnixNano()
	if now-p.lastactive.Load() > int64(heart) {
		//heartbeat timeout
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.checkheart] heart timeout", slog.String("cip", p.c.RemoteAddr().String()))
		} else {
			slog.ErrorContext(nil, "[Stream.checkheart] heart timeout", slog.String("sip", p.c.RemoteAddr().String()))
		}
		p.c.Close()
		return
	}
	if now-p.sendidlestart.Load() > int64(sendidle) {
		//send idle timeout
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.checkheart] send idle timeout", slog.String("cip", p.c.RemoteAddr().String()))
		} else {
			slog.ErrorContext(nil, "[Stream.checkheart] send idle timeout", slog.String("sip", p.c.RemoteAddr().String()))
		}
		p.c.Close()
		return
	}
	if recvidle > 0 && now-p.recvidlestart.Load() > int64(recvidle) {
		//recv idle timeout
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.checkheart] recv idle timeout", slog.String("cip", p.c.RemoteAddr().String()))
		} else {
			slog.ErrorContext(nil, "[Stream.checkheart] recv idle timeout", slog.String("sip", p.c.RemoteAddr().String()))
		}
		p.c.Close()
		return
	}
	//send heart probe data
	go func() {
		buf := bpool.Get(8)
		defer bpool.Put(&buf)
		buf = buf[:8]
		binary.BigEndian.PutUint64(buf, uint64(now))
		if e := ws.WritePing(p.c, buf, false); e != nil {
			if p.peertype == _PEER_CLIENT {
				slog.ErrorContext(nil, "[Stream.checkheart] write ping to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			} else {
				slog.ErrorContext(nil, "[Stream.checkheart] write ping to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			}
			p.c.Close()
			return
		}
		p.sendidlestart.Store(now)
	}()
}

func (p *Peer) getDispatcher(ctx context.Context) error {
	//first check
	if p.status.Load() != 1 {
		return ErrConnClosed
	}
	select {
	case _, ok := <-p.dispatcher:
		if !ok {
			return ErrConnClosed
		} else if p.status.Load() != 1 {
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
	if p.status.Load() == 1 {
		p.dispatcher <- nil
	} else {
		close(p.dispatcher)
	}
}

type BeforeSend func(*Peer)
type AfterSend func(*Peer, error)

// SendMessage will return ErrMsgLarge/ErrConnClosed/context.Canceled/context.DeadlineExceeded
// there may be lots of goroutines calling this function at the same time,but only one goroutine can be actived once,others need to be block and wait
// the bs(before send) will be called before the caller is really ready to send the data(it is not block now)
// the as(after send) will be called after the caller finishs the send
func (p *Peer) SendMessage(ctx context.Context, userdata []byte, bs BeforeSend, as AfterSend) error {
	if len(userdata) == 0 {
		return nil
	}
	if uint64(len(userdata)) > uint64(p.peerMaxMsgLen.Load()) {
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
			if p.peertype == _PEER_CLIENT {
				slog.ErrorContext(ctx, "[Stream.SendMessage] write to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			} else {
				slog.ErrorContext(ctx, "[Stream.SendMessage] write to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			}
			p.c.Close()
			if as != nil {
				as(p, e)
			}
			return ErrConnClosed
		}
		p.sendidlestart.Store(time.Now().UnixNano())
		first = false
	}
	if as != nil {
		as(p, nil)
	}
	return nil
}
func (p *Peer) SendPing() error {
	buf := bpool.Get(8)
	defer bpool.Put(&buf)
	buf = buf[:8]
	now := time.Now()
	binary.BigEndian.PutUint64(buf, uint64(now.UnixNano()))
	if e := ws.WritePing(p.c, buf, false); e != nil {
		return e
	}
	p.sendidlestart.Store(now.UnixNano())
	return nil
}

func (p *Peer) Close(block bool) {
	p.status.Store(0)
	p.c.Close()
	if block {
		<-p.blocknotice
	}
}

// 1-peer is a server,self is client,2-peer is a client,self is server
func (p *Peer) GetPeerType() int {
	return p.peertype
}

// if uniqueid return in verify callback function is empty,the peer's RemoteAddr(ip:port) will be returned
func (p *Peer) GetUniqueID() string {
	return p.uniqueid
}

func (p *Peer) GetLocalPort() string {
	laddr := p.c.LocalAddr().String()
	return laddr[strings.LastIndex(laddr, ":")+1:]
}

func (p *Peer) GetNetlag() int64 {
	return p.netlag.Load()
}

// only useful when peertype is _PEER_SERVER,this is the server's raw connect addr
func (p *Peer) GetRawConnectAddr() string {
	if p.peertype == _PEER_SERVER {
		return p.rawconnectaddr
	}
	return ""
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
	return p.peerMaxMsgLen.Load()
}

func (p *Peer) GetData() unsafe.Pointer {
	return atomic.LoadPointer(&p.data)
}

func (p *Peer) SetData(data unsafe.Pointer) {
	atomic.StorePointer(&p.data, data)
}
