package stream

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
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
		conn.SetReadBuffer(int(this.c.TcpC.SocketRBufLen))
		conn.SetWriteBuffer(int(this.c.TcpC.SocketWBufLen))
		if tlsc != nil {
			p.conn = tls.Server(conn, tlsc)
			p.tls = true
		} else {
			p.conn = conn
		}
		go this.sworker(p)
	}
}

func (this *Instance) sworker(p *Peer) {
	p.conn.SetReadDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
	p.conn.SetWriteDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
	ctx, cancel := context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
	defer cancel()
	if p.tls {
		if e := p.conn.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.sworker] tls handshake error:", e)
			p.conn.Close()
			return
		}
	}
	//read first verify message from client
	verifydata := this.verifypeer(ctx, p, false)
	if p.peeruniquename == "" {
		p.conn.Close()
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.sworker] add:", p.peeruniquename, "to connection manager error:", e)
		p.conn.Close()
		return
	}
	//verify client success,send self's verify message to client
	verifymsg := makeVerifyMsg(this.selfappname, verifydata, p.selfmaxmsglen)
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
	(conn.(*net.TCPConn)).SetReadBuffer(int(this.c.TcpC.SocketRBufLen))
	(conn.(*net.TCPConn)).SetWriteBuffer(int(this.c.TcpC.SocketWBufLen))
	p := newPeer(this.c.TcpC.MaxMsgLen)
	if tlsc != nil {
		p.conn = tls.Client(conn, tlsc)
		p.tls = true
	} else {
		p.conn = conn
	}
	return this.cworker(p, verifydata, dl)
}

func (this *Instance) cworker(p *Peer, verifydata []byte, dl time.Time) bool {
	p.conn.SetReadDeadline(dl)
	p.conn.SetWriteDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if p.tls {
		if e := p.conn.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.cworker] tls handshake error:", e)
			p.conn.Close()
			return false
		}
	}
	//send self's verify message to server
	verifymsg := makeVerifyMsg(this.selfappname, verifydata, p.selfmaxmsglen)
	defer bufpool.PutBuffer(verifymsg)
	if _, e := p.conn.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.cworker] write verify msg to:", p.conn.RemoteAddr().String(), "error:", e)
		p.conn.Close()
		return false
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p, true)
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

func (this *Instance) verifypeer(ctx context.Context, p *Peer, clientorserver bool) []byte {
	data, e := p.readMessage()
	var sender string
	var peerverifydata []byte
	var peermaxmsglength uint32
	if e == nil {
		if data == nil {
			e = ErrMsgEmpty
		} else {
			defer bufpool.PutBuffer(data)
			var msgtype int
			msgtype, peerverifydata, sender, peermaxmsglength, e = decodeMsg(data.Bytes())
			if e == nil && msgtype != VERIFY {
				e = ErrMsgUnknown
			}
		}
	}
	if e != nil {
		if clientorserver {
			log.Error(nil, "[Stream.verifypeer] read msg from server:", p.conn.RemoteAddr(), "error:", e)
		} else {
			log.Error(nil, "[Stream.verifypeer] read msg from client:", p.conn.RemoteAddr(), "error:", e)
		}
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	dup := make([]byte, len(sender))
	copy(dup, sender)
	p.peeruniquename = common.Byte2str(dup) + ":" + p.conn.RemoteAddr().String()
	p.peermaxmsglen = peermaxmsglength
	response, success := this.c.Verifyfunc(ctx, p.peeruniquename, peerverifydata)
	if !success {
		if clientorserver {
			log.Error(nil, "[Stream.verifypeer] verify server:", p.peeruniquename, "failed")
		} else {
			log.Error(nil, "[Stream.verifypeer] verify client:", p.peeruniquename, "failed")
		}
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
	for {
		var msgtype int
		var userdata []byte
		data, e := p.readMessage()
		if e == nil {
			if data == nil {
				e = ErrMsgEmpty
			} else {
				msgtype, userdata, _, _, e = decodeMsg(data.Bytes())
				if e == nil && msgtype != PING && msgtype != PONG && msgtype != USER {
					e = ErrMsgUnknown
				}
			}
		}
		if e != nil {
			log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error:", e)
			if data != nil {
				bufpool.PutBuffer(data)
			}
			return
		}
		//deal message
		switch msgtype {
		case PING:
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, time.Now().UnixNano())
			//write back
			if len(userdata) > 0 {
				pongdata := make([]byte, len(userdata))
				copy(pongdata, userdata)
				go p.sendPong(pongdata)
			} else {
				go p.sendPong(nil)
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
		case PONG:
			if len(userdata) != 8 {
				log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error: pong data format")
				bufpool.PutBuffer(data)
				return
			}
			sendtime := binary.BigEndian.Uint64(userdata)
			if sendtime >= uint64(math.MaxInt64) {
				log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error: pong data format")
				bufpool.PutBuffer(data)
				return
			}
			netlag := time.Now().UnixNano() - int64(sendtime)
			if netlag < 0 {
				log.Error(nil, "[Stream.handle] from:", p.peeruniquename, "error: pong data format")
				bufpool.PutBuffer(data)
				return
			}
			//update lastactive time
			atomic.StoreInt64(&p.lastactive, time.Now().UnixNano())
			atomic.StoreInt64(&p.netlag, netlag)
			binary.BigEndian.Uint64(userdata)
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p)
			}
		case USER:
			//update lastactive time
			now := time.Now().UnixNano()
			atomic.StoreInt64(&p.lastactive, now)
			atomic.StoreInt64(&p.recvidlestart, now)
			this.c.Userdatafunc(p, userdata)
		}
		bufpool.PutBuffer(data)
	}
}
