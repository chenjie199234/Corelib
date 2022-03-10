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
)

var ErrServerClosed = errors.New("[Stream.server] closed")

//listenaddr is ip:port
//if tlsc not nil,tcp connection will be used with tls
func (this *Instance) StartServer(listenaddr string, tlsc *tls.Config) error {
	if tlsc != nil && len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
		return errors.New("[Stream.StartServer] tls certificate setting missing")
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream.StartServer] resolve tcp addr: " + listenaddr + " error: " + e.Error())
	}
	this.Lock()
	if this.mng.Finishing() {
		this.Unlock()
		return ErrServerClosed
	}
	var tmplistener *net.TCPListener
	if tmplistener, e = net.ListenTCP("tcp", laddr); e != nil {
		this.Unlock()
		return errors.New("[Stream.StartServer] listen tcp addr: " + listenaddr + " error: " + e.Error())
	}
	this.listeners = append(this.listeners, tmplistener)
	this.Unlock()
	for {
		p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_CLIENT)
		conn, e := tmplistener.AcceptTCP()
		if e != nil {
			if ee, ok := e.(interface {
				Temporary() bool
			}); ok && ee.Temporary() {
				log.Error(nil, "[Stream.StartServer] accept with temporary error:", ee)
				continue
			}
			tmplistener.Close()
			if this.mng.Finishing() {
				return ErrServerClosed
			}
			return errors.New("[Stream.StartServer] accept error: " + e.Error())
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
			p.cr = pool.GetReader(p.c)
		} else {
			p.c = conn
			p.cr = pool.GetReader(p.c)
		}
		p.realPeerIP = conn.RemoteAddr().String()[:strings.LastIndex(conn.RemoteAddr().String(), ":")]
		p.c.SetDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		ctx, cancel := context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
		go func() {
			defer cancel()
			if tlsc != nil {
				//tls handshake
				if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
					log.Error(nil, "[Stream.StartServer] from:", p.c.RemoteAddr().String(), "tls handshake error:", e)
					p.c.Close()
					return
				}
			}
			if tmp, e := p.cr.Peek(1); e != nil {
				log.Error(nil, "[Stream.StartServer] from:", p.c.RemoteAddr().String(), "read error:", e)
				p.c.Close()
				return
			} else if tmp[0] == 'G' {
				//G -> 1000111,opcode is 7,doesn't exist this opcode,so this is a websocket handshake
				p.ws = true
				if !this.wsServerHandshake(p) {
					p.c.Close()
					return
				}
			}
			this.sworker(ctx, p)
		}()
	}
}

func (this *Instance) sworker(ctx context.Context, p *Peer) {
	//read first verify message from client
	serververifydata := this.verifypeer(ctx, p)
	if p.peermaxmsglen == 0 {
		p.c.Close()
		return
	}
	if 4+uint64(len(serververifydata)) > uint64(p.peermaxmsglen) {
		log.Error(nil, "[Stream.sworker] server verify data too large for:", p.c.RemoteAddr().String())
		p.c.Close()
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.sworker] add:", p.c.RemoteAddr().String(), "to connection manager error:", e)
		p.c.Close()
		return
	}
	//verify client success,send self's verify message to client
	verifymsg := makeVerifyMsg(p.selfmaxmsglen, serververifydata, false)
	defer pool.PutBuffer(verifymsg)
	if _, e := p.c.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.sworker] write verify msg to:", p.c.RemoteAddr().String(), "error:", e)
		p.c.Close()
		this.mng.DelPeer(p)
		return
	}
	//verify finished,status set to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(p) {
			log.Error(nil, "[Stream.sworker] online:", p.c.RemoteAddr().String(), "failed")
			atomic.StoreInt32(&p.status, 0)
			p.c.Close()
			this.mng.DelPeer(p)
			p.CancelFunc()
			return
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return
}

//if rawtcp client,serveraddr is [tcp/tcps]://ip:port
//if websocket client,serveraddr is [ws/wss]://host:port/path
func (this *Instance) StartClient(serveraddr string, verifydata []byte, tlsc *tls.Config) bool {
	if 4+uint64(len(verifydata)) > uint64(math.MaxUint32) {
		log.Error(nil, "[Stream.StartClient] client verify data too large")
		return false
	}
	u, e := url.Parse(serveraddr)
	if e != nil {
		log.Error(nil, "[Stream.StartClient] server addr format error:", e)
		return false
	}
	if u.Scheme != "tcp" && u.Scheme != "tcps" && u.Scheme != "ws" && u.Scheme != "wss" {
		log.Error(nil, "[Stream.StartClient] unknown scheme:", u.Scheme)
		return false
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
		log.Error(nil, "[Stream.StartClient] dial error:", e)
		return false
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := newPeer(this.c.TcpC.MaxMsgLen, _PEER_SERVER)
	if u.Scheme == "tcps" || u.Scheme == "wss" {
		p.c = tls.Client(conn, tlsc)
		p.cr = pool.GetReader(p.c)
	} else {
		p.c = conn
		p.cr = pool.GetReader(p.c)
	}
	p.realPeerIP = conn.RemoteAddr().String()[:strings.LastIndex(conn.RemoteAddr().String(), ":")]
	p.c.SetDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if u.Scheme == "tcps" || u.Scheme == "wss" {
		//tls handshake
		if e := p.c.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.StartClient] to:", serveraddr, "tls handshake error:", e)
			p.c.Close()
			return false
		}
	}
	if u.Scheme == "ws" || u.Scheme == "wss" {
		p.ws = true
		if !wsClientHandshake(p, u) {
			//ws handshake
			p.c.Close()
			return false
		}
	}
	return this.cworker(ctx, p, verifydata)
}

func (this *Instance) cworker(ctx context.Context, p *Peer, clientverifydata []byte) bool {
	//send self's verify message to server
	verifymsg := makeVerifyMsg(p.selfmaxmsglen, clientverifydata, p.ws)
	defer pool.PutBuffer(verifymsg)
	if _, e := p.c.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.cworker] write verify msg to:", p.c.RemoteAddr().String(), "error:", e)
		p.c.Close()
		return false
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.peermaxmsglen == 0 {
		p.c.Close()
		return false
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.cworker] add:", p.c.RemoteAddr().String(), "to connection manager error:", e)
		p.c.Close()
		return false
	}
	//verify finished set status to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.OnlineFunc != nil {
		if !this.c.OnlineFunc(p) {
			log.Error(nil, "[Stream.cworker] online:", p.c.RemoteAddr().String(), "failed")
			atomic.StoreInt32(&p.status, 0)
			p.c.Close()
			this.mng.DelPeer(p)
			p.CancelFunc()
			return false
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.c.SetDeadline(time.Time{})
	go this.handle(p)
	return true
}

func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	//first msg must be verify msg
	//verify msg must be fin
	fin, opcode, e := p.readMessage(nil, buf)
	if e != nil {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "error:", e)
		return nil
	}
	if !fin || iscontrol(opcode) {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "error: verify msg format wrong")
		return nil
	}
	senderMaxRecvMsgLen, senderverifydata := getVerifyMsg(buf.Bytes())
	if senderverifydata == nil {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "error: verify msg format wrong")
		return nil
	}
	if senderMaxRecvMsgLen < 65535 {
		log.Error(nil, "[Stream.verifypeer] from:", p.c.RemoteAddr().String(), "error: maxmsglen too small")
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	p.peermaxmsglen = senderMaxRecvMsgLen
	response, success := this.c.VerifyFunc(ctx, senderverifydata)
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
		pool.PutReader(p.cr)
	}()
	//before handle user data,send first ping,to get the net lag
	if e := p.sendPing(time.Now().UnixNano()); e != nil {
		log.Error(nil, "[Stream.handle] send first ping to:", p.c.RemoteAddr().String(), "error:", e)
		return
	}
	var total *pool.Buffer
	for {
		tmp := pool.GetBuffer()
		fin, opcode, e := p.readMessage(total, tmp)
		if e != nil {
			log.Error(nil, "[Stream.handle] from:", p.c.RemoteAddr().String(), "error:", e)
			pool.PutBuffer(total)
			pool.PutBuffer(tmp)
			total = nil
			return
		}
		switch opcode {
		case _CLOSE:
			pool.PutBuffer(total)
			pool.PutBuffer(tmp)
			total = nil
			return
		case _PING:
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, time.Now().UnixNano())
			//write back
			p.sendPong(tmp)
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			pool.PutBuffer(tmp)
		case _PONG:
			if tmp.Len() != 8 {
				log.Error(nil, "[Stream.handle.pong] from:", p.c.RemoteAddr().String(), "error: format wrong")
				pool.PutBuffer(total)
				pool.PutBuffer(tmp)
				total = nil
				return
			}
			sendtime := binary.BigEndian.Uint64(tmp.Bytes())
			now := time.Now()
			netlag := now.UnixNano() - int64(sendtime)
			if netlag < 0 {
				log.Error(nil, "[Stream.handle.pong] from:", p.c.RemoteAddr().String(), "error: format wrong")
				pool.PutBuffer(total)
				pool.PutBuffer(tmp)
				total = nil
				return
			}
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, now.UnixNano())
			atomic.StoreInt64(&p.netlag, netlag)
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			pool.PutBuffer(tmp)
		default:
			//update lastactive time
			now := time.Now().UnixNano()
			atomic.StoreInt64(&p.lastactive, now)
			atomic.StoreInt64(&p.recvidlestart, now)
			if fin {
				if total == nil {
					this.c.UserdataFunc(p, tmp.Bytes())
				} else {
					this.c.UserdataFunc(p, total.Bytes())
				}
				pool.PutBuffer(total)
				pool.PutBuffer(tmp)
				total = nil
			} else if total == nil {
				total = tmp
			}
		}
	}
}
