package stream

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gobwas/ws"
)

func (this *Instance) StartTcpServer(listenaddr string) error {
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream] resolve tcp addr:" + listenaddr + " error:" + e.Error())
	}
	this.tcplistener, e = net.ListenTCP(laddr.Network(), laddr)
	if e != nil {
		return errors.New("[Stream] listen tcp addr:" + listenaddr + " error:" + e.Error())
	}
	for {
		p := this.getPeer(TCP, CLIENT, this.c.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen, this.selfname)
		conn, e := this.tcplistener.AcceptTCP()
		if e != nil {
			return errors.New("[Stream] accept tcp connection error:" + e.Error())
		}
		if atomic.LoadInt32(&this.stop) == 1 {
			conn.Close()
			this.putPeer(p)
			this.tcplistener.Close()
			return errors.New("[Stream] accept tcp connection error: server closed")
		}
		//disable system's keep alive probe
		//use self's heartbeat probe
		conn.SetKeepAlive(false)
		rc, _ := conn.SyscallConn()
		rc.Control(func(fd uintptr) {
			p.fd = uint64(fd)
		})
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(int(this.c.TcpC.SocketRBufLen), int(this.c.TcpC.SocketWBufLen))
		if p.reader == nil {
			p.reader = bufio.NewReaderSize(conn, int(this.c.TcpC.SocketRBufLen))
		} else {
			p.reader.Reset(conn)
		}
		go this.sworker(p, nil)
	}
}
func (this *Instance) StartUnixServer(listenaddr string) error {
	laddr, e := net.ResolveUnixAddr("unix", listenaddr)
	if e != nil {
		return errors.New("[Stream] resolve unix addr:" + listenaddr + " error:" + e.Error())
	}
	this.unixlistener, e = net.ListenUnix("unix", laddr)
	if e != nil {
		return errors.New("[Stream] listen unix addr:" + listenaddr + " error:" + e.Error())
	}
	for {
		p := this.getPeer(UNIX, CLIENT, this.c.MaxBufferedWriteMsgNum, this.c.UnixC.MaxMsgLen, this.selfname)
		conn, e := this.unixlistener.AcceptUnix()
		if e != nil {
			return errors.New("[Stream] accept unix connection error:" + e.Error())
		}
		if atomic.LoadInt32(&this.stop) == 1 {
			conn.Close()
			this.putPeer(p)
			this.unixlistener.Close()
			return errors.New("[Stream] accept unix connection error: server closed")
		}
		rc, _ := conn.SyscallConn()
		rc.Control(func(fd uintptr) {
			p.fd = uint64(fd)
		})
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(int(this.c.UnixC.SocketRBufLen), int(this.c.UnixC.SocketWBufLen))
		if p.reader == nil {
			p.reader = bufio.NewReaderSize(conn, int(this.c.UnixC.SocketRBufLen))
		} else {
			p.reader.Reset(conn)
		}
		go this.sworker(p, nil)
	}
}
func (this *Instance) StartWsServer(listenaddr, uri string) error {
	if uri == "" {
		uri = "/"
	}
	if uri[0] != '/' {
		uri = "/" + uri
	}
	upgrader := &ws.Upgrader{
		OnRequest: func(requri []byte) error {
			if common.Byte2str(requri) != uri {
				return ws.RejectConnectionError(
					ws.RejectionStatus(http.StatusNotFound),
					ws.RejectionReason("handshake error: 404 Not Found"),
				)
			}
			return nil
		},
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[Stream] resolve ws addr:" + listenaddr + " error:" + e.Error())
	}
	this.tcplistener, e = net.ListenTCP(laddr.Network(), laddr)
	if e != nil {
		return errors.New("[Stream] listen ws addr:" + listenaddr + " error:" + e.Error())
	}
	for {
		p := this.getPeer(WS, CLIENT, this.c.MaxBufferedWriteMsgNum, this.c.WsC.MaxMsgLen, this.selfname)
		conn, e := this.tcplistener.AcceptTCP()
		if e != nil {
			return errors.New("[Stream] accept ws connection error:" + e.Error())
		}
		if atomic.LoadInt32(&this.stop) == 1 {
			conn.Close()
			this.putPeer(p)
			this.tcplistener.Close()
			return errors.New("[Stream] accept ws connection error: server closed")
		}
		//disable system's keep alive probe
		//use self's heartbeat probe
		conn.SetKeepAlive(false)
		rc, _ := conn.SyscallConn()
		rc.Control(func(fd uintptr) {
			p.fd = uint64(fd)
		})
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(int(this.c.WsC.SocketRBufLen), int(this.c.WsC.SocketWBufLen))
		if p.reader == nil {
			p.reader = bufio.NewReaderSize(conn, int(this.c.WsC.SocketRBufLen))
		} else {
			p.reader.Reset(conn)
		}
		go this.sworker(p, upgrader)
	}
}

func (this *Instance) sworker(p *Peer, u *ws.Upgrader) {
	var ctx context.Context
	var cancel context.CancelFunc
	switch p.protocol {
	case WS:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Now().Add(this.c.WsC.ConnectTimeout))
		(*net.TCPConn)(p.conn).SetWriteDeadline(time.Now().Add(this.c.WsC.ConnectTimeout))
		if _, e := u.Upgrade((*net.TCPConn)(p.conn)); e != nil {
			log.Error("[Stream.sworker] upgrade websocket error:", e)
			return
		}
		ctx, cancel = context.WithTimeout(p, this.c.WsC.ConnectTimeout)
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		(*net.TCPConn)(p.conn).SetWriteDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		ctx, cancel = context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Now().Add(this.c.UnixC.ConnectTimeout))
		(*net.UnixConn)(p.conn).SetWriteDeadline(time.Now().Add(this.c.UnixC.ConnectTimeout))
		ctx, cancel = context.WithTimeout(p, this.c.UnixC.ConnectTimeout)
	}
	defer cancel()
	//read first verify message from client
	verifydata := this.verifypeer(ctx, p)
	if p.clientname == "" {
		p.closeconn()
		this.putPeer(p)
		return
	}
	if !this.addPeer(p) {
		log.Error("[Stream.sworker] dup", p.protocol.protoname(), "connection from client:", p.getUniqueName())
		p.closeconn()
		this.putPeer(p)
		return
	}
	if atomic.LoadInt32(&this.stop) == 1 {
		p.closeconn()
		//after addpeer should use this way to delete this peer
		this.noticech <- p
		return
	}
	//verify client success,send self's verify message to client
	var verifymsg *bufpool.Buffer
	if p.protocol == WS {
		verifymsg = makeVerifyMsg(p.servername, verifydata, p.sid, false)
	} else {
		verifymsg = makeVerifyMsg(p.servername, verifydata, p.sid, true)
	}
	defer bufpool.PutBuffer(verifymsg)
	var e error
	switch p.protocol {
	case WS:
		e = ws.WriteFrame((*net.TCPConn)(p.conn), ws.NewBinaryFrame(verifymsg.Bytes()))
	case TCP:
		_, e = (*net.TCPConn)(p.conn).Write(verifymsg.Bytes())
	case UNIX:
		_, e = (*net.UnixConn)(p.conn).Write(verifymsg.Bytes())
	}
	if e != nil {
		log.Error("[Stream.sworker] write verify msg to", p.protocol.protoname(), "client:", p.getUniqueName(), "error:", e)
		p.closeconn()
		//after addpeer should use this way to delete this peer
		this.noticech <- p
		return
	}
	if this.c.Onlinefunc != nil {
		this.c.Onlinefunc(p, p.getUniqueName(), p.sid)
	}
	go this.read(p)
	go this.write(p)
	return
}

// success return peeruniquename
// fail return empty
func (this *Instance) StartTcpClient(serveraddr string, verifydata []byte) string {
	if atomic.LoadInt32(&this.stop) == 1 {
		return ""
	}
	dialer := net.Dialer{
		Timeout:   this.c.TcpC.ConnectTimeout,
		KeepAlive: -time.Second, //disable system's tcp keep alive probe,use self's heartbeat probe
		Control: func(network, address string, c syscall.RawConn) error {
			c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, int(this.c.TcpC.SocketRBufLen))
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, int(this.c.TcpC.SocketWBufLen))
			})
			return nil
		},
	}
	dl := time.Now().Add(this.c.TcpC.ConnectTimeout)
	conn, e := dialer.Dial("tcp", serveraddr)
	if e != nil {
		log.Error("[Stream] dial tcp server error:", e)
		return ""
	}
	//disable system's tcp keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := this.getPeer(TCP, SERVER, this.c.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen, this.selfname)
	rc, _ := conn.(*net.TCPConn).SyscallConn()
	rc.Control(func(fd uintptr) {
		p.fd = uint64(fd)
	})
	p.conn = unsafe.Pointer(conn.(*net.TCPConn))
	p.setbuffer(int(this.c.TcpC.SocketRBufLen), int(this.c.TcpC.SocketWBufLen))
	if p.reader == nil {
		p.reader = bufio.NewReaderSize(conn, int(this.c.TcpC.SocketRBufLen))
	} else {
		p.reader.Reset(conn)
	}
	return this.cworker(p, verifydata, dl)
}

// success return peeruniquename
// fail return empty
func (this *Instance) StartUnixClient(serveraddr string, verifydata []byte) string {
	if atomic.LoadInt32(&this.stop) == 1 {
		return ""
	}
	dialer := net.Dialer{
		Timeout: this.c.UnixC.ConnectTimeout,
		Control: func(network, address string, c syscall.RawConn) error {
			c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, int(this.c.UnixC.SocketRBufLen))
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, int(this.c.UnixC.SocketWBufLen))
			})
			return nil
		},
	}
	dl := time.Now().Add(this.c.UnixC.ConnectTimeout)
	conn, e := dialer.Dial("unix", serveraddr)
	if e != nil {
		log.Error("[Stream] dial unix server error:", e)
		return ""
	}
	p := this.getPeer(UNIX, SERVER, this.c.MaxBufferedWriteMsgNum, this.c.UnixC.MaxMsgLen, this.selfname)
	rc, _ := conn.(*net.UnixConn).SyscallConn()
	rc.Control(func(fd uintptr) {
		p.fd = uint64(fd)
	})
	p.conn = unsafe.Pointer(conn.(*net.UnixConn))
	p.setbuffer(int(this.c.UnixC.SocketRBufLen), int(this.c.UnixC.SocketWBufLen))
	if p.reader == nil {
		p.reader = bufio.NewReaderSize(conn, int(this.c.UnixC.SocketRBufLen))
	} else {
		p.reader.Reset(conn)
	}
	return this.cworker(p, verifydata, dl)
}

// success return peeruniquename
// fail return empty
func (this *Instance) StartWsClient(serveraddr string, verifydata []byte) string {
	return ""
}
func (this *Instance) cworker(p *Peer, verifydata []byte, dl time.Time) string {
	var ctx context.Context
	var cancel context.CancelFunc
	switch p.protocol {
	case WS:
		fallthrough
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(dl)
		(*net.TCPConn)(p.conn).SetWriteDeadline(dl)
		ctx, cancel = context.WithDeadline(p, dl)
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadDeadline(dl)
		(*net.UnixConn)(p.conn).SetWriteDeadline(dl)
		ctx, cancel = context.WithDeadline(p, dl)
	}
	defer cancel()
	//send self's verify message to server
	var verifymsg *bufpool.Buffer
	if p.protocol == WS {
		verifymsg = makeVerifyMsg(p.clientname, verifydata, 0, false)
	} else {
		verifymsg = makeVerifyMsg(p.clientname, verifydata, 0, true)
	}
	defer bufpool.PutBuffer(verifymsg)
	send := 0
	num := 0
	var e error
	for send < verifymsg.Len() {
		switch p.protocol {
		case TCP:
			num, e = (*net.TCPConn)(p.conn).Write(verifymsg.Bytes()[send:])
		case UNIX:
			num, e = (*net.UnixConn)(p.conn).Write(verifymsg.Bytes()[send:])
		}
		if e != nil {
			log.Error("[Stream.cworker] write verify msg to", p.protocol.protoname(), "server:", p.getUniqueName(), "error:", e)
			p.closeconn()
			this.putPeer(p)
			return ""
		}
		send += num
	}
	//read first verify message from server
	_ = this.verifypeer(ctx, p)
	if p.servername == "" {
		p.closeconn()
		this.putPeer(p)
		return ""
	}
	//verify server success
	if !this.addPeer(p) {
		log.Error("[Stream.cworker] dup", p.protocol.protoname(), "connection to server:", p.getUniqueName())
		p.closeconn()
		this.putPeer(p)
		return ""
	}
	if atomic.LoadInt32(&this.stop) == 1 {
		p.closeconn()
		//after addpeer should use this way to delete this peer
		this.noticech <- p
		return ""
	}
	if this.c.Onlinefunc != nil {
		this.c.Onlinefunc(p, p.getUniqueName(), p.sid)
	}
	uniquename := p.getUniqueName()
	go this.read(p)
	go this.write(p)
	return uniquename
}
func (this *Instance) verifypeer(ctx context.Context, p *Peer) []byte {
	data, e := p.readMessage()
	var sender string
	var peerverifydata []byte
	var sid int64
	if e == nil {
		if data == nil {
			e = errors.New("empty message")
		} else {
			defer bufpool.PutBuffer(data)
			var msgtype int
			if msgtype, e = getMsgType(data.Bytes()); e == nil {
				if msgtype != VERIFY {
					e = errors.New("first msg is not verify msg")
				} else if sender, peerverifydata, sid, e = getVerifyMsg(data.Bytes()); e == nil && (sender == p.clientname || sender == p.servername) {
					e = errors.New("sender name illegal")
				}
			}
		}
	}
	if e != nil {
		log.Error("[Stream.verifypeer] read msg from", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName(), "error:", e)
		return nil
	}
	p.lastactive = time.Now().UnixNano()
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	dup := make([]byte, len(sender))
	copy(dup, sender)
	switch p.peertype {
	case CLIENT:
		p.clientname = common.Byte2str(dup)
		p.sid = p.lastactive
	case SERVER:
		p.servername = common.Byte2str(dup)
		p.sid = sid
	}
	response, success := this.c.Verifyfunc(ctx, p.getUniqueName(), peerverifydata)
	if !success {
		log.Error("[Stream.verifypeer]", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName(), "verify failed")
		p.clientname = ""
		p.servername = ""
		return nil
	}
	return response
}
func (this *Instance) read(p *Peer) {
	defer func() {
		//every connection will have two goroutine to work for it
		if atomic.SwapInt64(&p.sid, 0) != 0 {
			//when first goroutine return,close the connection,cause write goroutine return
			p.closeconn()
			//wake up write goroutine,because it may block on channel
			select {
			case p.pingpongbuffer <- (*bufpool.Buffer)(nil):
			default:
			}
		} else {
			if this.c.Offlinefunc != nil {
				this.c.Offlinefunc(p, p.getUniqueName())
			}
			//when second goroutine return,put connection back to the pool
			this.noticech <- p
		}
	}()
	//after verify,the conntimeout is useless,heartbeat will work for this
	switch p.protocol {
	case WS:
		fallthrough
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Time{})
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Time{})
	}
	for {
		var msgtype int
		data, e := p.readMessage()
		if e == nil {
			if data == nil {
				e = errors.New("empty message")
			} else {
				msgtype, e = getMsgType(data.Bytes())
				if e == nil && msgtype != PING && msgtype != PONG && msgtype != WSPING && msgtype != WSPONG && msgtype != USER {
					e = errors.New("unknown msg type")
				}
			}
		}
		if e != nil {
			log.Error("[Stream.read] from", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName(), "error:", e)
			if data != nil {
				bufpool.PutBuffer(data)
			}
			return
		}
		//deal message
		switch msgtype {
		case WSPING:
			//update lastactive time
			p.lastactive = time.Now().UnixNano()
			//write back
			pingdata, _ := getPingMsg(data.Bytes())
			pongdata := makeWsPongMsg(pingdata)
			p.pingpongbuffer <- pongdata
		case PING:
			//update lastactive time
			p.lastactive = time.Now().UnixNano()
			//write back
			pingdata, _ := getPingMsg(data.Bytes())
			var pongdata *bufpool.Buffer
			if p.protocol == WS {
				pongdata = makePongMsg(pingdata, false)
			} else {
				pongdata = makePongMsg(pingdata, true)
			}
			p.pingpongbuffer <- pongdata
		case WSPONG:
			fallthrough
		case PONG:
			//update lastactive time
			p.lastactive = time.Now().UnixNano()
		case USER:
			userdata, sid, _ := getUserMsg(data.Bytes())
			log.Info(sid)
			log.Info(p.sid)
			if sid == p.sid {
				//update lastactive time
				p.lastactive = time.Now().UnixNano()
				p.recvidlestart = p.lastactive
				this.c.Userdatafunc(p, p.getUniqueName(), userdata, sid)
			}
			//drop race data
		}
		bufpool.PutBuffer(data)
	}
}

func (this *Instance) write(p *Peer) {
	defer func() {
		//drop all data,prevent close read goruntine block on send empty data to these channel
		for len(p.writerbuffer) > 0 {
			if v := <-p.writerbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		for len(p.pingpongbuffer) > 0 {
			if v := <-p.pingpongbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		//every connection will have two goroutine to work for it
		if atomic.SwapInt64(&p.sid, 0) != 0 {
			//when first goroutine return,close the connection,cause read goroutine return
			p.closeconn()
		} else {
			if this.c.Offlinefunc != nil {
				this.c.Offlinefunc(p, p.getUniqueName())
			}
			//when second goroutine return,put connection back to the pool
			this.noticech <- p
		}
	}()
	//after verify,the conntimeout is useless,heartbeat will work for this
	switch p.protocol {
	case WS:
		fallthrough
	case TCP:
		(*net.TCPConn)(p.conn).SetWriteDeadline(time.Time{})
	case UNIX:
		(*net.UnixConn)(p.conn).SetWriteDeadline(time.Time{})
	}
	var data *bufpool.Buffer
	var ok bool
	var e error
	for {
		if atomic.LoadInt64(&p.sid) <= 0 && len(p.writerbuffer) == 0 {
			return
		}
		if len(p.pingpongbuffer) > 0 {
			data, ok = <-p.pingpongbuffer
		} else {
			select {
			case data, ok = <-p.writerbuffer:
			case data, ok = <-p.pingpongbuffer:
			}
		}
		if !ok || data == nil {
			continue
		}
		p.sendidlestart = time.Now().UnixNano()
		switch p.protocol {
		case WS:
			t, _ := getMsgType(data.Bytes())
			if t == WSPING {
				//ws ping
				pingdata, _ := getPingMsg(data.Bytes())
				e = ws.WriteFrame((*net.TCPConn)(p.conn), ws.NewPingFrame(pingdata))
			} else if t == WSPONG {
				//ws pong
				pongdata, _ := getPongMsg(data.Bytes())
				e = ws.WriteFrame((*net.TCPConn)(p.conn), ws.NewPongFrame(pongdata))
			} else {
				e = ws.WriteFrame((*net.TCPConn)(p.conn), ws.NewBinaryFrame(data.Bytes()))
			}
		case TCP:
			_, e = (*net.TCPConn)(p.conn).Write(data.Bytes())
		case UNIX:
			_, e = (*net.UnixConn)(p.conn).Write(data.Bytes())
		}
		if e != nil {
			log.Error("[Stream.write] to", p.protocol.protoname(), p.peertype.typename()+":", p.getUniqueName(), "error:", e)
			return
		}
		bufpool.PutBuffer(data)
	}
}
