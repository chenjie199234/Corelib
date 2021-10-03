package stream

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

var ErrServerClosed = errors.New("[Stream.server] closed")
var ErrAlreadyStarted = errors.New("[Stream.server] already started")

//if tlsc not nil,tcp connection will be used with tls
func (this *Instance) StartTcpServer(listenaddr string, tlsc *tls.Config) error {
	if tlsc != nil && len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
		return errors.New("[Stream.StartTcpServer] tls certificate setting missing")
	}
	//check status
	if this.mng.Finishing() {
		return ErrServerClosed
	}
	this.Lock()
	if this.tcplistener != nil {
		this.Unlock()
		return ErrAlreadyStarted
	}
	var e error
	if this.tcplistener, e = net.Listen("tcp", listenaddr); e != nil {
		this.Unlock()
		return errors.New("[Stream.StartTcpServer] listen tcp addr: " + listenaddr + " error: " + e.Error())
	}
	this.Unlock()
	//double check stop status
	if this.mng.Finishing() {
		this.tcplistener.Close()
		return ErrServerClosed
	}
	for {
		p := this.getPeer(this.c.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen)
		conn, e := this.tcplistener.Accept()
		if e != nil {
			this.putPeer(p)
			this.tcplistener.Close()
			if this.mng.Finishing() {
				return ErrServerClosed
			}
			return errors.New("[Stream.server] accept error: " + e.Error())
		}
		if this.mng.Finishing() {
			conn.Close()
			this.putPeer(p)
			this.tcplistener.Close()
			return ErrServerClosed
		}
		//disable system's keep alive probe
		//use self's heartbeat probe
		(conn.(*net.TCPConn)).SetKeepAlive(false)
		(conn.(*net.TCPConn)).SetReadBuffer(int(this.c.TcpC.SocketRBufLen))
		(conn.(*net.TCPConn)).SetWriteBuffer(int(this.c.TcpC.SocketWBufLen))
		if tlsc != nil {
			p.conn = tls.Server(conn, tlsc)
			p.rawconn = conn
			p.tls = true
		} else {
			p.conn = conn
			p.rawconn = conn
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
			this.putPeer(p)
			return
		}
	}
	//read first verify message from client
	verifydata := this.verifypeer(ctx, p, false)
	if p.peername == "" {
		p.conn.Close()
		this.putPeer(p)
		return
	}
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.sworker] add:", p.getUniqueName(), "to peer manager error:", e)
		p.conn.Close()
		this.putPeer(p)
		return
	}
	if this.c.Onlinefunc != nil {
		if !this.c.Onlinefunc(p, p.getUniqueName(), p.sid) {
			log.Error(nil, "[Stream.sworker] online:", p.getUniqueName(), "failed")
			p.conn.Close()
			//after addpeer should use this way to delete this peer
			this.noticech <- p
			return
		}
	}
	//verify client success,send self's verify message to client
	verifymsg := makeVerifyMsg(this.selfname, verifydata, p.sid, p.selfmaxmsglen)
	defer bufpool.PutBuffer(verifymsg)
	if _, e := p.conn.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.sworker] write verify msg to:", p.getUniqueName(), "error:", e)
		p.conn.Close()
		//after addpeer should use this way to delete this peer
		this.noticech <- p
		return
	}
	go this.read(p)
	go this.write(p)
	return
}

// success return peeruniquename
// fail return empty
//if tlsc not nil,tcp connection will be used with tls
func (this *Instance) StartTcpClient(serveraddr string, verifydata []byte, tlsc *tls.Config) string {
	if this.mng.Finishing() {
		return ""
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
		return ""
	}
	//disable system's keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	(conn.(*net.TCPConn)).SetReadBuffer(int(this.c.TcpC.SocketRBufLen))
	(conn.(*net.TCPConn)).SetWriteBuffer(int(this.c.TcpC.SocketWBufLen))
	p := this.getPeer(this.c.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen)
	if tlsc != nil {
		p.conn = tls.Client(conn, tlsc)
		p.rawconn = conn
		p.tls = true
	} else {
		p.conn = conn
		p.rawconn = conn
	}
	return this.cworker(p, verifydata, dl)
}

func (this *Instance) cworker(p *Peer, verifydata []byte, dl time.Time) string {
	p.conn.SetReadDeadline(dl)
	p.conn.SetWriteDeadline(dl)
	ctx, cancel := context.WithDeadline(p, dl)
	defer cancel()
	if p.tls {
		if e := p.conn.(*tls.Conn).HandshakeContext(ctx); e != nil {
			log.Error(nil, "[Stream.cworker] tls handshake error:", e)
			p.conn.Close()
			return ""
		}
	}
	//send self's verify message to server
	verifymsg := makeVerifyMsg(this.selfname, verifydata, 0, p.selfmaxmsglen)
	defer bufpool.PutBuffer(verifymsg)
	if _, e := p.conn.Write(verifymsg.Bytes()); e != nil {
		log.Error(nil, "[Stream.cworker] write verify msg to:", p.getUniqueName(), "error:", e)
		p.conn.Close()
		this.putPeer(p)
		return ""
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p, true)
	if p.peername == "" {
		p.conn.Close()
		this.putPeer(p)
		return ""
	}
	//verify server success
	if e := this.mng.AddPeer(p); e != nil {
		log.Error(nil, "[Stream.cworker] add:", p.getUniqueName(), "to peer manager error:", e)
		p.conn.Close()
		this.putPeer(p)
		return ""
	}
	if this.c.Onlinefunc != nil {
		if !this.c.Onlinefunc(p, p.getUniqueName(), p.sid) {
			log.Error(nil, "[Stream.cworker] online:", p.getUniqueName(), "failed")
			p.conn.Close()
			this.noticech <- p
			return ""
		}
	}
	uniquename := p.getUniqueName()
	go this.read(p)
	go this.write(p)
	return uniquename
}

var errSelfConnect = errors.New("peer name same as self")

func (this *Instance) verifypeer(ctx context.Context, p *Peer, clientorserver bool) []byte {
	data, e := p.readMessage()
	var sender string
	var peerverifydata []byte
	var peermaxmsglength uint32
	var sid int64
	if e == nil {
		if data == nil {
			e = ErrMsgEmpty
		} else {
			defer bufpool.PutBuffer(data)
			var msgtype int
			if msgtype, e = getMsgType(data.Bytes()); e == nil {
				if msgtype != VERIFY {
					e = ErrMsgUnknown
				} else if sender, peerverifydata, sid, peermaxmsglength, e = getVerifyMsg(data.Bytes()); e == nil && sender == this.selfname {
					e = errSelfConnect
				}
			}
		}
	}
	if e != nil {
		if clientorserver {
			log.Error(nil, "[Stream.verifypeer] read msg from server:", p.getUniqueName(), "error:", e)
		} else {
			log.Error(nil, "[Stream.verifypeer] read msg from client:", p.getUniqueName(), "error:", e)
		}
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	dup := make([]byte, len(sender))
	copy(dup, sender)
	p.peername = common.Byte2str(dup)
	p.peermaxmsglen = peermaxmsglength
	if clientorserver {
		//self is client
		p.sid = sid
	} else {
		//self is server
		p.sid = p.lastactive
	}
	response, success := this.c.Verifyfunc(ctx, p.getUniqueName(), peerverifydata)
	if !success {
		if clientorserver {
			log.Error(nil, "[Stream.verifypeer] verify server:", p.getUniqueName(), "failed")
		} else {
			log.Error(nil, "[Stream.verifypeer] verify client:", p.getUniqueName(), "failed")
		}
		p.peername = ""
		return nil
	}
	return response
}
func (this *Instance) read(p *Peer) {
	defer func() {
		p.rawconn.(*net.TCPConn).CloseRead()
		//every connection will have two goroutine to work for it
		if atomic.SwapInt64(&p.sid, -1) == -1 {
			//when second goroutine return,put connection back to the pool
			this.noticech <- p
		} else {
			//wake up write goroutine
			select {
			case p.pingpongbuffer <- (*Msg)(nil):
			default:
			}
		}
	}()
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.conn.SetReadDeadline(time.Time{})
	for {
		var msgtype int
		data, e := p.readMessage()
		if e == nil {
			if data == nil {
				e = ErrMsgEmpty
			} else {
				msgtype, e = getMsgType(data.Bytes())
				if e == nil && msgtype != PING && msgtype != PONG && msgtype != USER {
					e = ErrMsgUnknown
				}
			}
		}
		if e != nil {
			log.Error(nil, "[Stream.read] from:", p.getUniqueName(), "error:", e)
			if data != nil {
				bufpool.PutBuffer(data)
			}
			return
		}
		//deal message
		switch msgtype {
		case PING:
			//update lastactive time
			p.lastactive = time.Now().UnixNano()
			//write back
			pingdata, _ := getPingMsg(data.Bytes())
			select {
			case p.pingpongbuffer <- &Msg{mtype: PONG, data: pingdata}:
			default:
			}
			if this.c.PingPongFunc != nil {
				this.c.PingPongFunc(p, p.getUniqueName(), pingdata, p.sid)
			}
		case PONG:
			//update lastactive time
			p.lastactive = time.Now().UnixNano()
			if this.c.PingPongFunc != nil {
				pongdata, _ := getPongMsg(data.Bytes())
				this.c.PingPongFunc(p, p.getUniqueName(), pongdata, p.sid)
			}
		case USER:
			userdata, sid, _ := getUserMsg(data.Bytes())
			//drop race data
			if sid == p.sid {
				//update lastactive time
				p.lastactive = time.Now().UnixNano()
				p.recvidlestart = p.lastactive
				this.c.Userdatafunc(p, p.getUniqueName(), userdata, sid)
			}
		}
		bufpool.PutBuffer(data)
	}
}

func (this *Instance) write(p *Peer) {
	defer func() {
		p.rawconn.(*net.TCPConn).CloseWrite()
		//every connection will have two goroutine to work for it
		if atomic.SwapInt64(&p.sid, -1) == -1 {
			//when second goroutine return,put connection back to the pool
			this.noticech <- p
		}
	}()
	//after verify,the conntimeout is useless,heartbeat will work for this
	p.conn.SetWriteDeadline(time.Time{})
	var msg *Msg
	var ok bool
	var data *bufpool.Buffer
	for {
		if atomic.LoadInt64(&p.sid) <= 0 && len(p.writerbuffer) == 0 {
			return
		}
		if len(p.pingpongbuffer) > 0 {
			msg, ok = <-p.pingpongbuffer
		} else {
			select {
			case msg, ok = <-p.writerbuffer:
			case msg, ok = <-p.pingpongbuffer:
			}
		}
		if !ok || msg == nil {
			continue
		}
		switch msg.mtype {
		case PING:
			data = makePingMsg(msg.data)
		case PONG:
			data = makePongMsg(msg.data)
		case USER:
			data = makeUserMsg(msg.data, msg.sid)
		default:
			continue
		}
		p.sendidlestart = time.Now().UnixNano()
		if _, e := p.conn.Write(data.Bytes()); e != nil {
			p.lastunsend = msg
			log.Error(nil, "[Stream.write] to:", p.getUniqueName(), "error:", e)
			return
		}
		msg.send = true
		bufpool.PutBuffer(data)
	}
}
