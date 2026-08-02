package stream

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

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
	config         *InstanceConfig
	uniqueid       string //if this is empty,the uniqueid will be setted with the peer's RemoteAddr(ip:port)
	blocknotice    chan struct{}
	peerMaxMsgLen  atomic.Uint32
	peergroup      *group
	status         atomic.Int32 //1 - working,0 - closed
	dispatcher     chan struct{}
	cr             *bufio.Reader
	c              net.Conn
	rawconnectaddr string //only useful when peertype is _PEER_SERVER,this is the server's raw connect addr
	peertype       int
	header         http.Header    //if this is not nil,means this is a websocket peer
	lastactive     atomic.Int64   //unixnano timestamp
	netlag         atomic.Int64   //unixnano
	data           unsafe.Pointer //user data
	context.Context
	context.CancelFunc
}

// rawconnectaddr is only useful when peertype is _PEER_SERVER,this is the server's raw connect addr
func newPeer(c *InstanceConfig, peertype int, rawconnectaddr string) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{
		config:         c,
		blocknotice:    make(chan struct{}),
		rawconnectaddr: rawconnectaddr,
		peertype:       peertype,
		dispatcher:     make(chan struct{}, 1),
		Context:        ctx,
		CancelFunc:     cancel,
	}
	p.dispatcher <- struct{}{}
	return p
}

// now is the currnet timestamp(unit nanosecond)
func (p *Peer) checkheart(now int64) {
	if p.status.Load() != 1 {
		return
	}
	//give 1/3 heartprobe for net lag
	if now-p.lastactive.Load() > int64(p.config.HeartprobeInterval*3+p.config.HeartprobeInterval/3) {
		//heartbeat timeout
		if p.peertype == _PEER_CLIENT {
			slog.Error("[Stream.checkheart] heart timeout", slog.String("cip", p.c.RemoteAddr().String()))
		} else {
			slog.Error("[Stream.checkheart] heart timeout", slog.String("sip", p.c.RemoteAddr().String()))
		}
		p.c.Close()
		return
	}
	//send heart probe data
	go func() {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(now))
		if e := ws.WritePing(p.c, p.config.WriteTimeout, buf, false); e != nil {
			if p.peertype == _PEER_CLIENT {
				slog.Error("[Stream.checkheart] write ping to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			} else {
				slog.Error("[Stream.checkheart] write ping to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			}
			p.c.Close()
			return
		}
	}()
}

func (p *Peer) handle() {
	defer func() {
		p.status.Store(0)
		p.c.Close()
		if p.config.OfflineFunc != nil {
			p.config.OfflineFunc(p)
		}
		p.peergroup.mng.DelPeer(p)
		p.CancelFunc()
	}()
	//before handle user data,send first ping,to get the net lag
	if e := p.SendPing(); e != nil {
		if p.peertype == _PEER_CLIENT {
			slog.Error("[Stream.handle] send first ping to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.Error("[Stream.handle] send first ping to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
		return
	}
	if e := ws.Read(p.cr, p.c, p.config.ReadTimeout, p.config.IdleTimeout, p.config.MaxMsgLen, false, func(opcode ws.OPCode, data []byte) (readmore bool) {
		switch {
		case !opcode.IsControl():
			now := time.Now()
			p.lastactive.Store(now.UnixNano())
			p.config.UserdataFunc(p, data)
			return true
		case opcode.IsPing():
			now := time.Now()
			p.lastactive.Store(now.UnixNano())
			//write back
			if p.config.WriteTimeout > 0 {
				p.c.SetWriteDeadline(now.Add(p.config.WriteTimeout))
			}
			if e := ws.WritePong(p.c, 0, data, false); e != nil {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.handle] send pong to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				} else {
					slog.Error("[Stream.handle] send pong to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				}
				return false
			}
			if p.config.PingPongFunc != nil {
				p.config.PingPongFunc(p)
			}
			return true
		case opcode.IsPong():
			if len(data) != 8 {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.handle] client pong msg format wrong", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.Error("[Stream.handle] server pong msg format wrong", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			pingtime := binary.BigEndian.Uint64(data)
			now := time.Now().UnixNano()
			p.netlag.Store(now - int64(pingtime))
			if p.netlag.Load() < 0 {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.handle] client pong msg broken", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.Error("[Stream.handle] server pong msg broken", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			p.lastactive.Store(now)
			if p.config.PingPongFunc != nil {
				p.config.PingPongFunc(p)
			}
			return true
		default:
			//close
			return false
		}
	}); e != nil {
		if p.peertype == _PEER_CLIENT {
			slog.Error("[Stream.handle] read from client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.Error("[Stream.handle] read from server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
	}
}

func (p *Peer) getDispatcher(ctx context.Context) error {
	//first check
	if p.status.Load() != 1 {
		return ErrConnClosed
	}
	if ctx.Err() != nil {
		return ctx.Err()
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
		p.dispatcher <- struct{}{}
	} else {
		close(p.dispatcher)
	}
}

type BeforeSend func(*Peer)
type AfterSend func(*Peer, error)

// SendMessage will return ErrMsgLarge/ErrConnClosed/context.Canceled/context.DeadlineExceeded/os.ErrDeadlineExceeded
// there may be lots of goroutines calling this function at the same time,but only one goroutine can be actived once,other needs to be block and wait
// the bs(before send) will be called before the caller is ready to send the data(it is not block now)
// the as(after send) will be called after finish the send
func (p *Peer) SendMessage(ctx context.Context, userdata []byte, bs BeforeSend, as AfterSend) error {
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
	if len(userdata) <= maxPieceLen {
		if p.config.WriteTimeout > 0 {
			p.c.SetWriteDeadline(time.Now().Add(p.config.WriteTimeout))
		}
		if e := ws.WriteMsg(p.c, 0, userdata, true, true, false); e != nil {
			if p.peertype == _PEER_CLIENT {
				slog.ErrorContext(ctx, "[Stream.SendMessage] write to client failed",
					slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			} else {
				slog.ErrorContext(ctx, "[Stream.SendMessage] write to server failed",
					slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			}
			p.c.Close()
			if as != nil {
				as(p, e)
			}
			if errors.Is(e, os.ErrDeadlineExceeded) {
				return os.ErrDeadlineExceeded
			}
			return ErrConnClosed
		}
	} else {
		for i := 0; i < len(userdata); i += maxPieceLen {
			var data []byte
			if i+maxPieceLen < len(userdata) {
				data = userdata[i : i+maxPieceLen]
			} else {
				data = userdata[i:]
			}
			if p.config.WriteTimeout > 0 {
				p.c.SetWriteDeadline(time.Now().Add(p.config.WriteTimeout))
			}
			if e := ws.WriteMsg(p.c, 0, data, i+maxPieceLen >= len(userdata), i == 0, false); e != nil {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(ctx, "[Stream.SendMessage] write to client failed",
						slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				} else {
					slog.ErrorContext(ctx, "[Stream.SendMessage] write to server failed",
						slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				}
				p.c.Close()
				if as != nil {
					as(p, e)
				}
				if errors.Is(e, os.ErrDeadlineExceeded) {
					return os.ErrDeadlineExceeded
				}
				return ErrConnClosed
			}
		}
	}
	if as != nil {
		as(p, nil)
	}
	return nil
}

// SendPing will return ErrConnClosed/os.ErrDeadlineExceeded
func (p *Peer) SendPing() error {
	buf := make([]byte, 8)
	now := time.Now()
	binary.BigEndian.PutUint64(buf, uint64(now.UnixNano()))
	if p.config.WriteTimeout > 0 {
		//reset the write deadline
		p.c.SetWriteDeadline(now.Add(p.config.WriteTimeout))
	}
	if e := ws.WritePing(p.c, 0, buf, false); e != nil {
		if errors.Is(e, os.ErrDeadlineExceeded) {
			return os.ErrDeadlineExceeded
		}
		return ErrConnClosed
	}
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
