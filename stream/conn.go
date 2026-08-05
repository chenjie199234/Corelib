package stream

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"net"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/ws"
)

var ErrServerClosed = errors.New("[Stream.server] closed")

// listenaddr: host:port or ip:port
// 1.one addr can both support raw tcp and websocket connections
// 2.websocket's 'host','path','origin' etc which from http will be ignored,works just like a raw tcp connection
// 3.both raw tcp and websocket use websocket's data frame format to communicate with the client
// 4.websocket need websocket's handshake,raw tcp doesn't need
// 5.client's message can be masked or not masked,both can be supported
// 6.if tlsc is not nil,the tls will be activated
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
		p := newPeer(this.c, _PEER_CLIENT, "")
		conn, e := tmplistener.AcceptTCP()
		if e != nil {
			if ee, ok := e.(interface{ Temporary() bool }); ok && ee.Temporary() {
				slog.Error("[Stream.StartServer] accept tcp connection failed", slog.String("error", e.Error()))
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
		p.cr = bufio.NewReader(p.c)
		//the config's validate func guarantee the ConnectTimeout always > 0
		p.c.SetDeadline(time.Now().Add(this.c.ConnectTimeout))
		ctx, cancel := context.WithTimeout(p, this.c.ConnectTimeout)
		go func() {
			defer cancel()
			if tlsc != nil {
				//tls handshake
				if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
					slog.Error("[Stream.StartServer] tls handshake failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
					p.c.Close()
					return
				}
			}
			//both raw tcp and websocket use the websocket's data frame format
			//websocket need the handshake,so the first byte must be G(http method GET)->71->0b01000111
			//if this is a raw tcp connection,the first byte can't be 0b01000111,because the opcode doesn't exist
			//so we can check the first byte with G to decide the raw tcp or websocket
			_, header, e := ws.Supgrade(p.cr, p.c)
			if e != nil && e != ws.ErrNotWS {
				slog.Error("[Stream.StartServer] upgrade websocket failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				p.c.Close()
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
		p.CancelFunc()
		p.c.Close()
		return
	}
	if 4+uint64(len(serververifydata)) > uint64(p.peerMaxMsgLen.Load()) {
		slog.Error("[Stream.sworker] server response verify data too large", slog.String("cip", p.c.RemoteAddr().String()))
		p.CancelFunc()
		p.c.Close()
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		slog.Error("[Stream.sworker] add client to connection manager failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		p.CancelFunc()
		p.c.Close()
		return
	}
	//verify client success,send self's verify message to client
	buf := make([]byte, 4+len(serververifydata))
	binary.BigEndian.PutUint32(buf, p.config.MaxMsgLen)
	copy(buf[4:], serververifydata)
	first := true
	for len(buf) > 0 {
		var data []byte
		if len(buf) > maxPieceLen {
			data = buf[:maxPieceLen]
			buf = buf[maxPieceLen:]
		} else {
			data = buf
			buf = nil
		}
		//set timeout to 0,current timeout is controlled by ConnectTimeout
		if e := ws.WriteMsg(p.c, 0, data, buf == nil, first, false); e != nil {
			slog.Error("[Stream.sworker] write verify data to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			return
		}
		if buf == nil {
			break
		}
		first = false
	}
	//verify finished,status set to 1(working)
	p.status.Store(1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(ctx, p) {
			slog.Error("[Stream.sworker] online failed", slog.String("cip", p.c.RemoteAddr().String()))
			p.status.Store(0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			return
		}
	}
	//after verify,the conntimeout is useless,heartbeattimeout,idletimeout,readtimeout will be used
	p.c.SetDeadline(time.Time{})
	go p.handle()
}

// serveraddr: host:port or ip:port
// 1.both raw tcp and websocket use websocket's data frame format to communicate with the server
// 2.if tlsc is not nil,the tls will be activated
func (this *Instance) StartClient(serveraddr string, websocket bool, verifydata []byte, tlsc *tls.Config) bool {
	if 4+uint64(len(verifydata)) > uint64(math.MaxUint32) {
		slog.Error("[Stream.StartClient] client verify data too large")
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
	//the config's validate func guarantee the ConnectTimeout always > 0
	dl := time.Now().Add(this.c.ConnectTimeout)
	conn, e := (&net.Dialer{Deadline: dl}).Dial("tcp", serveraddr)
	if e != nil {
		slog.Error("[Stream.StartClient] dial failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
		return false
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := newPeer(this.c, _PEER_SERVER, serveraddr)
	if tlsc != nil {
		p.c = tls.Client(conn, tlsc)
	} else {
		p.c = conn
	}
	p.cr = bufio.NewReader(p.c)
	p.c.SetDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if tlsc != nil {
		//tls handshake
		if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
			slog.Error("[Stream.StartClient] tls handshake failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
			p.c.Close()
			return false
		}
	}
	if websocket {
		//websocket handshake
		header, e := ws.Cupgrade(p.cr, p.c, serveraddr, "/")
		if e != nil {
			slog.Error("[Stream.StartClient] upgrade websocket failed", slog.String("sip", serveraddr), slog.String("error", e.Error()))
			p.c.Close()
			return false
		}
		p.header = header
	}
	return this.cworker(ctx, p, verifydata)
}

func (this *Instance) cworker(ctx context.Context, p *Peer, clientverifydata []byte) bool {
	//send self's verify message to server
	buf := make([]byte, 4+len(clientverifydata))
	binary.BigEndian.PutUint32(buf, p.config.MaxMsgLen)
	copy(buf[4:], clientverifydata)
	first := true
	for len(buf) > 0 {
		var data []byte
		if len(buf) > maxPieceLen {
			data = buf[:maxPieceLen]
			buf = buf[maxPieceLen:]
		} else {
			data = buf
			buf = nil
		}
		//set timeout to 0,current timeout is controlled by ConnectTimeout
		if e := ws.WriteMsg(p.c, 0, data, buf == nil, first, false); e != nil {
			slog.Error("[Stream.cworker] write verify data to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
			p.CancelFunc()
			p.c.Close()
			return false
		}
		if buf == nil {
			break
		}
		first = false
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.uniqueid == "" {
		p.CancelFunc()
		p.c.Close()
		return false
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		slog.Error("[Stream.cworker] add server to connection manager failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		p.CancelFunc()
		p.c.Close()
		return false
	}
	//verify finished set status to true
	p.status.Store(1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(ctx, p) {
			slog.Error("[Stream.cworker] online failed", slog.String("sip", p.c.RemoteAddr().String()))
			p.status.Store(0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			return false
		}
	}
	//after verify,the conntimeout is useless,heartbeattimeout,idletimeout,readtimeout will be used
	p.c.SetDeadline(time.Time{})
	go p.handle()
	return true
}

func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	var response []byte
	//set timeout to 0,current timeout is controlled by ConnectTimeout
	if e := ws.Read(p.cr, p.c, 0, 0, p.config.MaxMsgLen, false, func(opcode ws.OPCode, data []byte) (readmore bool) {
		switch {
		case !opcode.IsControl():
			if len(data) < 4 {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.verifypeer] client verify data format wrong", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.Error("[Stream.verifypeer] server verify data format wrong", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			senderMaxRecvMsgLen := binary.BigEndian.Uint32(data[:4])
			if senderMaxRecvMsgLen < 65536 {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.verifypeer] client maxmsglen too small", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.Error("[Stream.verifypeer] server maxmsglen too small", slog.String("sip", p.c.RemoteAddr().String()))
				}
				return false
			}
			now := time.Now()
			p.lastactive.Store(now.UnixNano())
			p.peerMaxMsgLen.Store(senderMaxRecvMsgLen)
			r, u, success := this.c.VerifyFunc(ctx, data[4:])
			if !success {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.verifypeer] verify client failed", slog.String("cip", p.c.RemoteAddr().String()))
				} else {
					slog.Error("[Stream.verifypeer] verify server failed", slog.String("sip", p.c.RemoteAddr().String()))
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
			//set timeout to 0,current timeout is controlled by ConnectTimeout
			if e := ws.WritePong(p.c, 0, data, false); e != nil {
				if p.peertype == _PEER_CLIENT {
					slog.Error("[Stream.verifypeer] write pong to client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
				} else {
					slog.Error("[Stream.verifypeer] write pong to server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
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
			slog.Error("[Stream.verifypeer] read from client failed", slog.String("cip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		} else {
			slog.Error("[Stream.verifypeer] read from server failed", slog.String("sip", p.c.RemoteAddr().String()), slog.String("error", e.Error()))
		}
	}
	return response
}
