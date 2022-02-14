package stream

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
)

var ErrServerClosed = errors.New("[Stream.server] closed")

//if tlsc not nil,tcp connection will be used with tls
func (this *Instance) StartTcpServer(listenaddr string, tlsc *tls.Config) error {
	if tlsc != nil && len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
		return errors.New("[Stream.StartTcpServer] tls certificate setting missing")
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream.StartTcpServer] resolve tcp addr: " + listenaddr + " error: " + e.Error())
	}
	this.Lock()
	if this.mng.Finishing() {
		this.Unlock()
		return ErrServerClosed
	}
	var tmplistener *net.TCPListener
	if tmplistener, e = net.ListenTCP("tcp", laddr); e != nil {
		this.Unlock()
		return errors.New("[Stream.StartTcpServer] listen tcp addr: " + listenaddr + " error: " + e.Error())
	}
	this.listeners = append(this.listeners, tmplistener)
	this.Unlock()
	for {
		p := newPeer(this.c.TcpC.MaxMsgLen)
		conn, e := tmplistener.AcceptTCP()
		if e != nil {
			if ee, ok := e.(interface {
				Temporary() bool
			}); ok && ee.Temporary() {
				log.Error(nil, "[Stream.StartTcpServer] accept with temporary error:", ee)
				continue
			}
			tmplistener.Close()
			if this.mng.Finishing() {
				return ErrServerClosed
			}
			return errors.New("[Stream.server] accept error: " + e.Error())
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
			p.conn = tls.Server(conn, tlsc)
		} else {
			p.conn = conn
		}
		go this.sworker(p, tlsc != nil)
	}
}

func (this *Instance) sworker(p *Peer, usetls bool) {
	p.conn.SetReadDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
	p.conn.SetWriteDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
	ctx, cancel := context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
	defer cancel()
	if usetls {
		if e := p.conn.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.sworker] tls handshake error:", e)
			p.conn.Close()
			return
		}
	}
	//read first verify message from client
	verifydata := this.verifypeer(ctx, p)
	if p.peeruniquename == "" {
		p.conn.Close()
		return
	}
	if len(verifydata)+len(this.selfappname) > maxPieceLen {
		log.Error(nil, "[Stream.sworker] self verify data too large")
		p.conn.Close()
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.sworker] add:", p.peeruniquename, "to connection manager error:", e)
		p.conn.Close()
		return
	}
	//verify client success,send self's verify message to client
	verifymsg := makeVerifyMsg(this.selfappname, p.selfmaxmsglen, verifydata)
	defer bufpool.PutBuffer(verifymsg)
	if _, e := p.conn.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.sworker] write verify msg to:", p.peeruniquename, "error:", e)
		p.conn.Close()
		this.mng.DelPeer(p)
		return
	}
	//verify finished,status set to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.Onlinefunc != nil {
		if !this.c.Onlinefunc(p) {
			log.Error(nil, "[Stream.sworker] online:", p.peeruniquename, "failed")
			atomic.StoreInt32(&p.status, 0)
			p.conn.Close()
			this.mng.DelPeer(p)
			p.CancelFunc()
			return
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.conn.SetReadDeadline(time.Time{})
	p.conn.SetWriteDeadline(time.Time{})
	go this.handle(p)
	return
}

//if tlsc not nil,tcp connection will be used with tls
func (this *Instance) StartTcpClient(serveraddr string, verifydata []byte, tlsc *tls.Config) bool {
	if len(this.selfappname)+len(verifydata) > maxPieceLen {
		log.Error(nil, "[Stream.StartTcpClient] self verify data too large")
		return false
	}
	if this.mng.Finishing() {
		return false
	}
	if tlsc != nil && tlsc.ServerName == "" {
		tlsc = tlsc.Clone()
		if index := strings.LastIndex(serveraddr, ":"); index == -1 {
			tlsc.ServerName = serveraddr
		} else {
			tlsc.ServerName = serveraddr[:index]
		}
	}
	dl := time.Now().Add(this.c.TcpC.ConnectTimeout)
	conn, e := (&net.Dialer{Deadline: dl}).Dial("tcp", serveraddr)
	if e != nil {
		log.Error(nil, "[Stream.StartTcpClient] dial error:", e)
		return false
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := newPeer(this.c.TcpC.MaxMsgLen)
	if tlsc != nil {
		p.conn = tls.Client(conn, tlsc)
	} else {
		p.conn = conn
	}
	return this.cworker(p, tlsc != nil, verifydata, dl)
}

func (this *Instance) cworker(p *Peer, usetls bool, verifydata []byte, dl time.Time) bool {
	p.conn.SetReadDeadline(dl)
	p.conn.SetWriteDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if usetls {
		if e := p.conn.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.cworker] tls handshake error:", e)
			p.conn.Close()
			return false
		}
	}
	//send self's verify message to server
	verifymsg := makeVerifyMsg(this.selfappname, p.selfmaxmsglen, verifydata)
	defer bufpool.PutBuffer(verifymsg)
	if _, e := p.conn.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.cworker] write verify msg to:", p.conn.RemoteAddr().String(), "error:", e)
		p.conn.Close()
		return false
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.peeruniquename == "" {
		p.conn.Close()
		return false
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.cworker] add:", p.peeruniquename, "to connection manager error:", e)
		p.conn.Close()
		return false
	}
	//verify finished set status to true
	atomic.StoreInt32(&p.status, 1)
	if this.c.Onlinefunc != nil {
		if !this.c.Onlinefunc(p) {
			log.Error(nil, "[Stream.cworker] online:", p.peeruniquename, "failed")
			atomic.StoreInt32(&p.status, 0)
			p.conn.Close()
			this.mng.DelPeer(p)
			p.CancelFunc()
			return false
		}
	}
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.conn.SetReadDeadline(time.Time{})
	p.conn.SetWriteDeadline(time.Time{})
	go this.handle(p)
	return true
}

func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	buf := bufpool.GetBuffer()
	defer bufpool.PutBuffer(buf)
	_, mtype, e := p.readMessage(nil, buf)
	if e != nil {
		log.Error(nil, "[Stream.verifypeer] from:", p.conn.RemoteAddr(), "error:", e)
		return nil
	}
	if mtype != _VERIFY {
		log.Error(nil, "[Stream.verifypeer] from:", p.conn.RemoteAddr(), "error: not verify msg")
		return nil
	}
	sender, sendermaxmsglen, senderverifydata, e := getVerifyMsg(buf.Bytes())
	if e != nil {
		log.Error(nil, "[Stream.verifypeer] from:", p.conn.RemoteAddr(), "error: format wrong")
		return nil
	}
	if sendermaxmsglen < 65535 {
		log.Error(nil, "[Stream.verifypeer] from:", sender+":"+p.conn.RemoteAddr().String(), "error: maxmsglen too small")
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	p.peeruniquename = sender + ":" + p.conn.RemoteAddr().String()
	p.peermaxmsglen = sendermaxmsglen
	response, success := this.c.Verifyfunc(ctx, p.peeruniquename, senderverifydata)
	if !success {
		log.Error(nil, "[Stream.verifypeer] verify:", p.peeruniquename, "failed")
		p.peeruniquename = ""
		return nil
	}
	return response
}
func (this *Instance) handle(p *Peer) {
	defer func() {
		atomic.StoreInt32(&p.status, 0)
		p.conn.Close()
		if this.c.Offlinefunc != nil {
			this.c.Offlinefunc(p)
		}
		this.mng.DelPeer(p)
		p.CancelFunc()
	}()
	pingdata := make([]byte, 8)
	binary.BigEndian.PutUint64(pingdata, uint64(time.Now().UnixNano()))
	//before handle user data,send first ping,to get the net lag
	if e := p.sendPing(pingdata); e != nil {
		log.Error(nil, "[Stream.handle] send first ping to:", p.peeruniquename, "error:", e)
		return
	}
	var total *bufpool.Buffer
	for {
		tmp := bufpool.GetBuffer()
		fin, mtype, e := p.readMessage(total, tmp)
		if e != nil {
			log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error:", e)
			bufpool.PutBuffer(total)
			bufpool.PutBuffer(tmp)
			total = nil
			return
		}
		switch mtype {
		case _PING:
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, time.Now().UnixNano())
			//write back
			p.sendPong(tmp)
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			bufpool.PutBuffer(tmp)
		case _PONG:
			if tmp.Len() != 8 {
				log.Error(nil, "[Stream.handle.pong] from:", p.peeruniquename, "error: format wrong")
				bufpool.PutBuffer(total)
				bufpool.PutBuffer(tmp)
				total = nil
				return
			}
			sendtime := binary.BigEndian.Uint64(tmp.Bytes())
			now := time.Now()
			netlag := now.UnixNano() - int64(sendtime)
			if netlag < 0 {
				log.Error(nil, "[Stream.handle.pong] from:", p.peeruniquename, "error: format wrong")
				bufpool.PutBuffer(total)
				bufpool.PutBuffer(tmp)
				total = nil
				return
			}
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, now.UnixNano())
			atomic.StoreInt64(&p.netlag, netlag)
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
			bufpool.PutBuffer(tmp)
		case _USER:
			//update lastactive time
			now := time.Now().UnixNano()
			atomic.StoreInt64(&p.lastactive, now)
			atomic.StoreInt64(&p.recvidlestart, now)
			if fin {
				if total == nil {
					this.c.Userdatafunc(p, tmp.Bytes())
				} else {
					this.c.Userdatafunc(p, total.Bytes())
				}
				bufpool.PutBuffer(total)
				bufpool.PutBuffer(tmp)
				total = nil
			} else if total == nil {
				total = tmp
			}
		default:
			log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error: message type wrong")
			bufpool.PutBuffer(total)
			bufpool.PutBuffer(tmp)
			total = nil
			return
		}
	}
}
