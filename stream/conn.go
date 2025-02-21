package stream

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/pool/rpool"
	"github.com/chenjie199234/Corelib/ws"
)

var ErrServerClosed = errors.New("[Stream.server] closed")

// listenaddr: host:port or ip:port
// 1.one addr can both support raw tcp and websocket connections
// 2.websocket's 'host','path','origin' etc which from http will be ignored,works just like a raw tcp connection
// 3.both raw tcp and websocket use websocket's data frame format to communicate with the client
// 4.websocket need websocket's handshake,raw tcp doesn't need
// 5.client's message can be masked or not masked,both can be supported
// 6.if tlsc is not nil,the tls will be actived
func (this *Instance) StartServer(listenaddr string, tlsc *tls.Config) error {
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return errors.New("[Stream.StartServer] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream.StartServer] resolve tcp addr: " + listenaddr + " error:" + e.Error())
	}
	this.Lock()
	if this.mng.Finishing() {
		this.Unlock()
		return ErrServerClosed
	}
	var tmplistener *net.TCPListener
	if tmplistener, e = net.ListenTCP("tcp", laddr); e != nil {
		this.Unlock()
		return errors.New("[Stream.StartServer] listen tcp addr: " + listenaddr + " error:" + e.Error())
	}
	this.listeners = append(this.listeners, tmplistener)
	this.Unlock()
	for {
		p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_CLIENT, "")
		conn, e := tmplistener.AcceptTCP()
		if e != nil {
			if ee, ok := e.(interface{ Temporary() bool }); ok && ee.Temporary() {
				slog.ErrorContext(nil, "[Stream.StartServer] accept tcp connection failed", slog.String("error", e.Error()))
				continue
			}
			tmplistener.Close()
			if this.mng.Finishing() {
				return ErrServerClosed
			}
			return errors.New("[Stream.StartServer] accept: " + e.Error())
		}
		if this.mng.Finishing() {
			conn.Close()
			tmplistener.Close()
			return ErrServerClosed
		}
		//disable system's keep alive probe
		//use self's heartbeat probe
		conn.SetKeepAlive(false)
		if tlsc != nil {
			p.c = tls.Server(conn, tlsc)
		} else {
			p.c = conn
		}
		p.cr = rpool.Get(p.c)
		p.c.SetDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		ctx, cancel := context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
		go func() {
			defer cancel()
			if tlsc != nil {
				//tls handshake
				if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
					slog.ErrorContext(nil, "[Stream.StartServer] tls handshake failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
					p.c.Close()
					rpool.Put(p.cr)
					return
				}
			}
			//both raw tcp and websocket use the websocket's data frame format
			//websocket need the handshake,so the first byte must be G->71->0b01000111
			//if this is a raw tcp connection,the first byte can't be 0b01000111,because the opcode doesn't exist
			//so we can check the first byte with G to decide the raw tcp or websocket
			_, header, e := ws.Supgrade(p.cr, p.c)
			if e != nil && e != ws.ErrNotWS {
				slog.ErrorContext(nil, "[Stream.StartServer] upgrade websocket failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				p.c.Close()
				rpool.Put(p.cr)
				return
			}
			if e == nil {
				//this is a websocket
				p.header = header
			}
			this.sworker(ctx, p)
		}()
	}
}

func (this *Instance) sworker(ctx context.Context, p *Peer) {
	//read first verify message from client
	serververifydata := this.verifypeer(ctx, p)
	if p.uniqueid == "" {
		p.c.Close()
		rpool.Put(p.cr)
		return
	}
	if 4+uint64(len(serververifydata)) > uint64(p.peerMaxMsgLen.Load()) {
		slog.ErrorContext(nil, "[Stream.sworker] server response verify data too large", slog.String("cip", p.c.RemoteAddr().String()))
		p.c.Close()
		rpool.Put(p.cr)
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		slog.ErrorContext(nil, "[Stream.sworker] add client to connection manager failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		p.c.Close()
		rpool.Put(p.cr)
		return
	}
	//verify client success,send self's verify message to client
	buf := bpool.Get(4)
	defer bpool.Put(&buf)
	buf = buf[:4]
	binary.BigEndian.PutUint32(buf, p.selfMaxMsgLen.Load())
	if len(serververifydata) == 0 {
		if e := ws.WriteMsg(p.c, buf, true, true, false); e != nil {
			slog.ErrorContext(nil, "[Stream.sworker] write verify data to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			this.mng.DelPeer(p)
			p.Close(false)
			rpool.Put(p.cr)
			return
		}
	} else if e := ws.WriteMsg(p.c, buf, false, true, false); e != nil {
		slog.ErrorContext(nil, "[Stream.sworker] write verify data to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		this.mng.DelPeer(p)
		p.Close(false)
		rpool.Put(p.cr)
		return
	} else {
		for len(serververifydata) > 0 {
			var data []byte
			if len(serververifydata) > maxPieceLen {
				data = serververifydata[:maxPieceLen]
				serververifydata = serververifydata[maxPieceLen:]
			} else {
				data = serververifydata
				serververifydata = nil
			}
			if e := ws.WriteMsg(p.c, data, serververifydata == nil, false, false); e != nil {
				slog.ErrorContext(nil, "[Stream.sworker] write verify data to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				this.mng.DelPeer(p)
				p.c.Close()
				rpool.Put(p.cr)
				return
			}
			if serververifydata == nil {
				break
			}
		}
	}
	//verify finished,status set to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(ctx, p) {
			slog.ErrorContext(nil, "[Stream.sworker] online failed", slog.String("cip", p.c.RemoteAddr().String()))
			atomic.StoreInt32(&p.status, 0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			rpool.Put(p.cr)
			return
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return
}

// serveraddr: host:port or ip:port
// 1.both raw tcp and websocket use websocket's data frame format to communicate with the server
// 2.if tlsc is not nil,the tls will be actived
func (this *Instance) StartClient(serveraddr string, websocket bool, verifydata []byte, tlsc *tls.Config) bool {
	if 4+uint64(len(verifydata)) > uint64(math.MaxUint32) {
		slog.ErrorContext(nil, "[Stream.StartClient] client verify data too large")
		return false
	}
	if tlsc != nil {
		tlsc = tlsc.Clone()
		if tlsc.ServerName == "" {
			if index := strings.LastIndex(serveraddr, ":"); index == -1 {
				tlsc.ServerName = serveraddr
			} else {
				tlsc.ServerName = serveraddr[:index]
			}
		}
	}
	if this.mng.Finishing() {
		return false
	}
	dl := time.Now().Add(this.c.TcpC.ConnectTimeout)
	conn, e := (&net.Dialer{Deadline: dl}).Dial("tcp", serveraddr)
	if e != nil {
		slog.ErrorContext(nil, "[Stream.StartClient] dial failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
		return false
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_SERVER, serveraddr)
	if tlsc != nil {
		p.c = tls.Client(conn, tlsc)
	} else {
		p.c = conn
	}
	p.cr = rpool.Get(p.c)
	p.c.SetDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if tlsc != nil {
		//tls handshake
		if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
			slog.ErrorContext(nil, "[Stream.StartClient] tls handshake failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
			p.c.Close()
			rpool.Put(p.cr)
			return false
		}
	}
	if websocket {
		//websocket handshake
		header, e := ws.Cupgrade(p.cr, p.c, serveraddr, "/")
		if e != nil {
			slog.ErrorContext(nil, "[Stream.StartClient] upgrade websocket failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
			p.c.Close()
			rpool.Put(p.cr)
			return false
		}
		p.header = header
	}
	return this.cworker(ctx, p, verifydata)
}

func (this *Instance) cworker(ctx context.Context, p *Peer, clientverifydata []byte) bool {
	//send self's verify message to server
	buf := bpool.Get(4)
	defer bpool.Put(&buf)
	buf = buf[:4]
	binary.BigEndian.PutUint32(buf, p.selfMaxMsgLen.Load())
	if len(clientverifydata) == 0 {
		if e := ws.WriteMsg(p.c, buf, true, true, false); e != nil {
			slog.ErrorContext(nil, "[Stream.cworker] write verify data to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			p.c.Close()
			rpool.Put(p.cr)
			return false
		}
	} else if e := ws.WriteMsg(p.c, buf, false, true, false); e != nil {
		slog.ErrorContext(nil, "[Stream.cworker] write verify data to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		p.c.Close()
		rpool.Put(p.cr)
		return false
	} else {
		for len(clientverifydata) > 0 {
			var data []byte
			if len(clientverifydata) > maxPieceLen {
				data = clientverifydata[:maxPieceLen]
				clientverifydata = clientverifydata[maxPieceLen:]
			} else {
				data = clientverifydata
				clientverifydata = nil
			}
			if e := ws.WriteMsg(p.c, data, clientverifydata == nil, false, false); e != nil {
				slog.ErrorContext(nil, "[Stream.cworker] write verify data to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				p.c.Close()
				rpool.Put(p.cr)
				return false
			}
			if clientverifydata == nil {
				break
			}
		}
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.uniqueid == "" {
		p.c.Close()
		rpool.Put(p.cr)
		return false
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		slog.ErrorContext(nil, "[Stream.cworker] add server to connection manager failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		p.c.Close()
		rpool.Put(p.cr)
		return false
	}
	//verify finished set status to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(ctx, p) {
			slog.ErrorContext(nil, "[Stream.cworker] online failed", slog.String("sip", p.c.RemoteAddr().String()))
			atomic.StoreInt32(&p.status, 0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			rpool.Put(p.cr)
			return false
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return true
}

func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	var response []byte
	if e := ws.Read(p.cr, p.selfMaxMsgLen.Load(), false, func(opcode ws.OPCode, data []byte) (readmore bool) {
		switch {
		case !opcode.IsControl():
			if len(data) < 4 {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.verifypeer] client verify data format wrong", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.ErrorContext(nil, "[Stream.verifypeer] server verify data format wrong", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			senderMaxRecvMsgLen := binary.BigEndian.Uint32(data[:4])
			if senderMaxRecvMsgLen < 65536 {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.verifypeer] client maxmsglen too small", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.ErrorContext(nil, "[Stream.verifypeer] server maxmsglen too small", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			now := time.Now()
			p.lastactive.Store(now.UnixNano())
			p.recvidlestart.Store(now.UnixNano())
			p.sendidlestart.Store(now.UnixNano())
			p.peerMaxMsgLen.Store(senderMaxRecvMsgLen)
			r, u, success := this.c.VerifyFunc(ctx, data[4:])
			if !success {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.verifypeer] verify client failed", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.ErrorContext(nil, "[Stream.verifypeer] verify server failed", slog.String("sip", p.c.RemoteAddr().String()))
				}
			} else {
				response = r
				if u == "" {
					p.uniqueid = p.GetRemoteAddr()
				} else {
					p.uniqueid = u
				}
			}
			return false
		case opcode.IsPing():
			//this can be possible when:
			//server get a connection from other implement's client which will send a ping before verify
			//client connect to an other implement's server which will send a ping before verify
			//write back
			if e := ws.WritePong(p.c, data, false); e != nil {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.verifypeer] write pong to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				} else {
					slog.ErrorContext(nil, "[Stream.verifypeer] write pong to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				}
				return false
			}
			//continue to read the verify message
			return true
		default:
			//if this is a pong:
			//both client and server in this implement will not send ping before verify,so this is not impossible
			//need to close the connection
			//if this is a close:
			//need to close the connection
			return false
		}
	}); e != nil {
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.verifypeer] read from client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.ErrorContext(nil, "[Stream.verifypeer] read from server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
	}
	return response
}
func (this *Instance) handle(p *Peer) {
	defer func() {
		atomic.StoreInt32(&p.status, 0)
		p.c.Close()
		if this.c.OfflineFunc != nil {
			this.c.OfflineFunc(p)
		}
		this.mng.DelPeer(p)
		p.CancelFunc()
		rpool.Put(p.cr)
	}()
	//before handle user data,send first ping,to get the net lag
	buf := bpool.Get(8)
	buf = buf[:8]
	binary.BigEndian.PutUint64(buf, uint64(time.Now().UnixNano()))
	e := ws.WritePing(p.c, buf, false)
	bpool.Put(&buf)
	if e != nil {
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.handle] send first ping to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.ErrorContext(nil, "[Stream.handle] send first ping to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
		return
	}
	if e = ws.Read(p.cr, p.selfMaxMsgLen.Load(), false, func(opcode ws.OPCode, data []byte) (readmore bool) {
		switch {
		case !opcode.IsControl():
			now := time.Now()
			p.lastactive.Store(now.UnixNano())
			p.recvidlestart.Store(now.UnixNano())
			this.c.UserdataFunc(p, data)
			return true
		case opcode.IsPing():
			p.lastactive.Store(time.Now().UnixNano())
			//write back
			if e := ws.WritePong(p.c, data, false); e != nil {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.handle] send pong to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				} else {
					slog.ErrorContext(nil, "[Stream.handle] send pong to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				}
				return false
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			return true
		case opcode.IsPong():
			if len(data) != 8 {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.handle] client pong msg format wrong", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.ErrorContext(nil, "[Stream.handle] server pong msg format wrong", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			sendtime := binary.BigEndian.Uint64(data)
			p.lastactive.Store(time.Now().UnixNano())
			p.netlag.Store(p.lastactive.Load() - int64(sendtime))
			if p.netlag.Load() < 0 {
				if p.peertype == _PEER_CLIENT {
					slog.ErrorContext(nil, "[Stream.handle] client pong msg broken", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.ErrorContext(nil, "[Stream.handle] server pong msg broken", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			return true
		default:
			//close
			return false
		}
	}); e != nil {
		if p.peertype == _PEER_CLIENT {
			slog.ErrorContext(nil, "[Stream.handle] read from client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.ErrorContext(nil, "[Stream.handle] read from server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
	}
}
