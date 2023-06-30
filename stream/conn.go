package stream

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/ws"
)

var ErrServerClosed = errors.New("[Stream.server] closed")

// 1.one addr can both support raw tcp or websocket connections
// 2.websocket's 'host','path','origin' etc which from http will be ignored
// 3.both raw tcp or websocket use websocket's data frame format to communicate with the client
// 4.if tlsc is not nil,tls handshake is required
// 5.websocket need websocket's handshake,raw tcp doesn't need
// 6.message can be masked or not masked,both can be supported
func (this *Instance) StartServer(listenaddr string, tlsc *tls.Config) error {
	if tlsc != nil && len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
		return errors.New("[Stream.StartServer] tls certificate setting missing")
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream.StartServer] resolve tcp addr: " + listenaddr + " " + e.Error())
	}
	this.Lock()
	if this.mng.Finishing() {
		this.Unlock()
		return ErrServerClosed
	}
	var tmplistener *net.TCPListener
	if tmplistener, e = net.ListenTCP("tcp", laddr); e != nil {
		this.Unlock()
		return errors.New("[Stream.StartServer] listen tcp addr: " + listenaddr + " " + e.Error())
	}
	this.listeners = append(this.listeners, tmplistener)
	this.Unlock()
	for {
		p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_CLIENT, "")
		conn, e := tmplistener.AcceptTCP()
		if e != nil {
			if ee, ok := e.(interface {
				Temporary() bool
			}); ok && ee.Temporary() {
				log.Error(nil, "[Stream.StartServer] accept failed temporary:", ee)
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
		p.cr = pool.GetReader(p.c)
		p.c.SetDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		ctx, cancel := context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
		go func() {
			defer cancel()
			if tlsc != nil {
				//tls handshake
				if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
					log.Error(nil, "[Stream.StartServer] from:", p.c.RemoteAddr().String(), "tls handshake:", e)
					p.c.Close()
					p.cr.Reset(nil)
					pool.PutReader(p.cr)
					return
				}
			}
			//both raw tcp and websocket use the websocket's data frame format
			//websocket need the handshake,so the first byte must be G->71->0b01000111
			//if this is a raw tcp connection,the first byte can't be 0b01000111,because the opcode doesn't exist
			//so we can check the first byte with G to decide the raw tcp or websocket
			_, header, e := ws.Supgrade(p.cr, p.c)
			if e != nil && e != ws.ErrNotWS {
				log.Error(nil, "[Stream.StartServer] from:", p.c.RemoteAddr().String(), "try to upgrade websocket:", e)
				p.c.Close()
				p.cr.Reset(nil)
				pool.PutReader(p.cr)
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
	if p.peerMaxMsgLen == 0 {
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
		return
	}
	if 4+uint64(len(serververifydata)) > uint64(p.peerMaxMsgLen) {
		log.Error(nil, "[Stream.sworker] server verify data too large for:", p.c.RemoteAddr().String())
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.sworker] add:", p.c.RemoteAddr().String(), "to connection manager error:", e)
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
		return
	}
	//verify client success,send self's verify message to client
	tmp := pool.GetBuffer()
	defer pool.PutBuffer(tmp)
	tmp.Resize(4)
	binary.BigEndian.PutUint32(tmp.Bytes(), p.selfMaxMsgLen)
	if len(serververifydata) == 0 {
		if e := ws.WriteMsg(p.c, tmp.Bytes(), true, true, false); e != nil {
			log.Error(nil, "[Stream.sworker] write verify msg to:", p.c.RemoteAddr().String(), e)
			this.mng.DelPeer(p)
			p.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return
		}
	} else if e := ws.WriteMsg(p.c, tmp.Bytes(), false, true, false); e != nil {
		log.Error(nil, "[Stream.sworker] write verify msg to:", p.c.RemoteAddr().String(), e)
		this.mng.DelPeer(p)
		p.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
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
				log.Error(nil, "[Stream.sworker] write verify msg to:", p.c.RemoteAddr().String(), e)
				this.mng.DelPeer(p)
				p.c.Close()
				p.cr.Reset(nil)
				pool.PutReader(p.cr)
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
		if !this.c.OnlineFunc(p) {
			log.Error(nil, "[Stream.sworker] online:", p.c.RemoteAddr().String(), "failed")
			atomic.StoreInt32(&p.status, 0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return
}

// 1.raw tcp client,serveraddr's format is [tcp/tcps]://[host/ip]:port
// 2.websocket client,serveraddr's format is [ws/wss]://[host/ip]:port/path
// 3.both raw tcp or websocket use websocket's data frame format to communicate with the server
// 4.if tlsc is not nil,tls handshake is required
// 5.websocket need websocket's handshake,raw tcp doesn't need
// 6.message can be masked or not masked,both can be supported
func (this *Instance) StartClient(serveraddr string, verifydata []byte, tlsc *tls.Config) bool {
	if 4+uint64(len(verifydata)) > uint64(math.MaxUint32) {
		log.Error(nil, "[Stream.StartClient] client verify data too large")
		return false
	}
	u, e := url.Parse(serveraddr)
	if e != nil {
		log.Error(nil, "[Stream.StartClient] serveraddr format:", e)
		return false
	}
	if u.Scheme != "tcp" && u.Scheme != "tcps" && u.Scheme != "ws" && u.Scheme != "wss" {
		log.Error(nil, "[Stream.StartClient] unknown scheme:", u.Scheme)
		return false
	}
	if (u.Scheme == "tcps" || u.Scheme == "wss") && tlsc == nil {
		tlsc = &tls.Config{}
	}
	if this.mng.Finishing() {
		return false
	}
	if tlsc != nil && tlsc.ServerName == "" {
		tlsc = tlsc.Clone()
		if index := strings.LastIndex(u.Host, ":"); index == -1 {
			tlsc.ServerName = u.Host
		} else {
			tlsc.ServerName = u.Host[:index]
		}
	}
	dl := time.Now().Add(this.c.TcpC.ConnectTimeout)
	conn, e := (&net.Dialer{Deadline: dl}).Dial("tcp", u.Host)
	if e != nil {
		log.Error(nil, "[Stream.StartClient] dial:", e)
		return false
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_SERVER, u.Host)
	if u.Scheme == "tcps" || u.Scheme == "wss" {
		p.c = tls.Client(conn, tlsc)
	} else {
		p.c = conn
	}
	p.cr = pool.GetReader(p.c)
	p.c.SetDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if u.Scheme == "tcps" || u.Scheme == "wss" {
		//tls handshake
		if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.StartClient] to:", serveraddr, "tls handshake error:", e)
			p.c.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return false
		}
	}
	if u.Scheme == "ws" || u.Scheme == "wss" {
		//websocket handshake
		header, e := ws.Cupgrade(p.cr, p.c, u.Host, u.Path)
		if e != nil {
			log.Error(nil, "[Stream.StartClient] to:", serveraddr, "upgrade websocket error:", e)
			p.c.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return false
		}
		p.header = header
	}
	return this.cworker(ctx, p, verifydata)
}

func (this *Instance) cworker(ctx context.Context, p *Peer, clientverifydata []byte) bool {
	//send self's verify message to server
	tmp := pool.GetBuffer()
	defer pool.PutBuffer(tmp)
	tmp.Resize(4)
	binary.BigEndian.PutUint32(tmp.Bytes(), p.selfMaxMsgLen)
	if len(clientverifydata) == 0 {
		if e := ws.WriteMsg(p.c, tmp.Bytes(), true, true, false); e != nil {
			log.Error(nil, "[Stream.cworker] write verify msg to:", p.c.RemoteAddr().String(), e)
			p.c.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return false
		}
	} else if e := ws.WriteMsg(p.c, tmp.Bytes(), false, true, false); e != nil {
		log.Error(nil, "[Stream.cworker] write verify msg to:", p.c.RemoteAddr().String(), e)
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
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
				log.Error(nil, "[Stream.cworker] write verify msg to:", p.c.RemoteAddr().String(), e)
				p.c.Close()
				p.cr.Reset(nil)
				pool.PutReader(p.cr)
				return false
			}
			if clientverifydata == nil {
				break
			}
		}
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.peerMaxMsgLen == 0 {
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
		return false
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.cworker] add:", p.c.RemoteAddr().String(), "to connection manager error:", e)
		p.c.Close()
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
		return false
	}
	//verify finished set status to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(p) {
			log.Error(nil, "[Stream.cworker] online:", p.c.RemoteAddr().String(), "failed")
			atomic.StoreInt32(&p.status, 0)
			this.mng.DelPeer(p)
			p.CancelFunc()
			p.c.Close()
			p.cr.Reset(nil)
			pool.PutReader(p.cr)
			return false
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return true
}

func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	msgbuf := pool.GetBuffer()
	defer pool.PutBuffer(msgbuf)
	ctlbuf := pool.GetBuffer()
	defer pool.PutBuffer(ctlbuf)
	for {
		opcode, e := ws.Read(p.cr, msgbuf, p.selfMaxMsgLen, ctlbuf, false)
		if e != nil {
			log.Error(nil, "[Stream.verifypeer] read from:", p.c.RemoteAddr().String(), e)
			return nil
		}
		if !opcode.IsControl() {
			break
		}
		if opcode.IsPing() {
			//this can be possible when:
			//server get a connection from other implement's client which will send a ping first
			//client connect to a other implement's server which will send a ping first
			//we write back
			if e := ws.WritePong(p.c, ctlbuf.Bytes(), false); e != nil {
				log.Error(nil, "[Stream.verifypeer] write pong to:", p.c.RemoteAddr().String(), e)
				return nil
			}
		} else {
			//if this is a pong:
			//both client and server will not send ping before verify,so this is not impossible
			//need to close the connection
			//if this is a close:
			//need to close the connection
			return nil
		}
	}
	if msgbuf.Len() < 4 {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "verify msg format wrong")
		return nil
	}
	senderMaxRecvMsgLen := binary.BigEndian.Uint32(msgbuf.Bytes()[:4])
	if senderMaxRecvMsgLen < 65536 {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "maxmsglen too small")
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	p.peerMaxMsgLen = senderMaxRecvMsgLen
	response, success := this.c.VerifyFunc(ctx, msgbuf.Bytes()[4:])
	if !success {
		log.Error(nil, "[Stream.verifypeer] verify:", p.c.RemoteAddr().String(), "failed")
		return nil
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
		p.cr.Reset(nil)
		pool.PutReader(p.cr)
	}()
	//before handle user data,send first ping,to get the net lag
	ctlbuf := pool.GetBuffer()
	ctlbuf.Resize(8)
	binary.BigEndian.PutUint64(ctlbuf.Bytes(), uint64(time.Now().UnixNano()))
	if e := ws.WritePing(p.c, ctlbuf.Bytes(), false); e != nil {
		log.Error(nil, "[Stream.handle] send first ping to:", p.c.RemoteAddr().String(), e)
		pool.PutBuffer(ctlbuf)
		return
	}
	ctlbuf.Reset()
	msgbuf := pool.GetBuffer()
	for {
		opcode, e := ws.Read(p.cr, msgbuf, p.selfMaxMsgLen, ctlbuf, false)
		if e != nil {
			log.Error(nil, "[Stream.handle] read from:", p.c.RemoteAddr().String(), e)
			pool.PutBuffer(ctlbuf)
			pool.PutBuffer(msgbuf)
			return
		}
		if !opcode.IsControl() {
			now := time.Now()
			p.lastactive = now.UnixNano()
			p.recvidlestart = now.UnixNano()
			this.c.UserdataFunc(p, msgbuf.Bytes())
			msgbuf.Reset()
		} else if opcode.IsPing() {
			p.lastactive = time.Now().UnixNano()
			//write back
			if e := ws.WritePong(p.c, ctlbuf.Bytes(), false); e != nil {
				log.Error(nil, "[Stream.handle] send pong to:", p.c.RemoteAddr().String(), e)
				pool.PutBuffer(ctlbuf)
				pool.PutBuffer(msgbuf)
				return
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			ctlbuf.Reset()
		} else if opcode.IsPong() {
			if ctlbuf.Len() != 8 {
				log.Error(nil, "[Stream.handle] from:", p.c.RemoteAddr().String(), "pong message format wrong")
				pool.PutBuffer(ctlbuf)
				pool.PutBuffer(msgbuf)
				return
			}
			sendtime := binary.BigEndian.Uint64(ctlbuf.Bytes())
			p.lastactive = time.Now().UnixNano()
			p.netlag = p.lastactive - int64(sendtime)
			if p.netlag < 0 {
				log.Error(nil, "[Stream.handle] from:", p.c.RemoteAddr().String(), "pong message data broken")
				pool.PutBuffer(ctlbuf)
				pool.PutBuffer(msgbuf)
				return
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			ctlbuf.Reset()
		} else {
			//close
			pool.PutBuffer(ctlbuf)
			pool.PutBuffer(msgbuf)
			return
		}
	}
}
