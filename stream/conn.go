package stream

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
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
		p := this.getPeer(TCP, CLIENT, this.c.TcpC.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen, this.selfname)
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
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(int(this.c.TcpC.SocketRBufLen), int(this.c.TcpC.SocketWBufLen))
		if p.reader == nil {
			p.reader = bufio.NewReaderSize(conn, int(this.c.TcpC.SocketRBufLen))
		} else {
			p.reader.Reset(conn)
		}
		go this.sworker(p)
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
		p := this.getPeer(UNIX, CLIENT, this.c.UnixC.MaxBufferedWriteMsgNum, this.c.UnixC.MaxMsgLen, this.selfname)
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
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(int(this.c.UnixC.SocketRBufLen), int(this.c.UnixC.SocketWBufLen))
		if p.reader == nil {
			p.reader = bufio.NewReaderSize(conn, int(this.c.UnixC.SocketRBufLen))
		} else {
			p.reader.Reset(conn)
		}
		go this.sworker(p)
	}
}

func (this *Instance) sworker(p *Peer) {
	//read first verify message from client
	verifydata := this.verifypeer(p)
	if p.clientname == "" {
		p.closeconn()
		this.putPeer(p)
		return
	}
	if !this.addPeer(p) {
		log.Error("[Stream.sworker] dup", p.protocol.protoname(), "connection from client:", p.getpeeruniquename())
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
	verifymsg := makeVerifyMsg(p.servername, verifydata, p.starttime, true)
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
			log.Error("[Stream.sworker] write verify msg to", p.protocol.protoname(), "client:", p.getpeeruniquename(), "error:", e)
			bufpool.PutBuffer(verifymsg)
			p.closeconn()
			this.putPeer(p)
			return
		}
		send += num
	}
	bufpool.PutBuffer(verifymsg)
	if this.c.Onlinefunc != nil {
		this.c.Onlinefunc(p, p.getpeeruniquename(), p.starttime)
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
	conn, e := dialer.Dial("tcp", serveraddr)
	if e != nil {
		log.Error("[Stream] dial tcp server error:", e)
		return ""
	}
	//disable system's tcp keep alive probe
	//use self's heartbeat probe
	(conn.(*net.TCPConn)).SetKeepAlive(false)
	p := this.getPeer(TCP, SERVER, this.c.TcpC.MaxBufferedWriteMsgNum, this.c.TcpC.MaxMsgLen, this.selfname)
	p.conn = unsafe.Pointer(conn.(*net.TCPConn))
	p.setbuffer(int(this.c.TcpC.SocketRBufLen), int(this.c.TcpC.SocketWBufLen))
	if p.reader == nil {
		p.reader = bufio.NewReaderSize(conn, int(this.c.TcpC.SocketRBufLen))
	} else {
		p.reader.Reset(conn)
	}
	return this.cworker(p, verifydata)
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
	conn, e := dialer.Dial("unix", serveraddr)
	if e != nil {
		log.Error("[Stream] dial unix server error:", e)
		return ""
	}
	p := this.getPeer(UNIX, SERVER, this.c.UnixC.MaxBufferedWriteMsgNum, this.c.UnixC.MaxMsgLen, this.selfname)
	p.conn = unsafe.Pointer(conn.(*net.UnixConn))
	p.setbuffer(int(this.c.UnixC.SocketRBufLen), int(this.c.UnixC.SocketWBufLen))
	if p.reader == nil {
		p.reader = bufio.NewReaderSize(conn, int(this.c.UnixC.SocketRBufLen))
	} else {
		p.reader.Reset(conn)
	}
	return this.cworker(p, verifydata)
}

func (this *Instance) cworker(p *Peer, verifydata []byte) string {
	//send self's verify message to server
	verifymsg := makeVerifyMsg(p.clientname, verifydata, 0, true)
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
			log.Error("[Stream.cworker] write verify msg to", p.protocol.protoname(), "server:", p.getpeeraddr(), "error:", e)
			bufpool.PutBuffer(verifymsg)
			p.closeconn()
			this.putPeer(p)
			return ""
		}
		send += num
	}
	bufpool.PutBuffer(verifymsg)
	//read first verify message from server
	_ = this.verifypeer(p)
	if p.servername == "" {
		p.closeconn()
		this.putPeer(p)
		return ""
	}
	//verify server success
	if !this.addPeer(p) {
		log.Error("[Stream.cworker] dup", p.protocol.protoname(), "connection to server:", p.getpeeruniquename())
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
		this.c.Onlinefunc(p, p.getpeeruniquename(), p.starttime)
	}
	uniquename := p.getpeeruniquename()
	go this.read(p)
	go this.write(p)
	return uniquename
}
func (this *Instance) verifypeer(p *Peer) []byte {
	var ctx context.Context
	var cancel context.CancelFunc
	switch p.protocol {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Now().Add(this.c.TcpC.ConnectTimeout))
		ctx, cancel = context.WithTimeout(p, this.c.TcpC.ConnectTimeout)
	case UNIX:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Now().Add(this.c.UnixC.ConnectTimeout))
		ctx, cancel = context.WithTimeout(p, this.c.UnixC.ConnectTimeout)
	}
	defer cancel()
	data, e := p.readMessage()
	if data != nil {
		defer bufpool.PutBuffer(data)
	}
	var sender string
	var peerverifydata []byte
	var starttime uint64
	if e == nil {
		if data == nil {
			e = errors.New("empty message")
		} else {
			var msgtype int
			if msgtype, e = getMsgType(data.Bytes()); e == nil {
				if msgtype != VERIFY {
					e = errors.New("first msg is not verify msg")
				} else if sender, peerverifydata, starttime, e = getVerifyMsg(data.Bytes()); e == nil && (sender == p.clientname || sender == p.servername) {
					e = errors.New("sender name illegal")
				}
			}
		}
	}
	if e != nil {
		log.Error("[Stream.verifypeer] read verify msg from", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeraddr(), "error:", e)
		return nil
	}
	p.lastactive = uint64(time.Now().UnixNano())
	p.recvidlestart = p.lastactive
	p.sendidlestart = p.lastactive
	dup := make([]byte, len(sender))
	copy(dup, sender)
	switch p.peertype {
	case CLIENT:
		p.clientname = common.Byte2str(dup)
		p.starttime = p.lastactive
	case SERVER:
		p.servername = common.Byte2str(dup)
		p.starttime = starttime
	}
	response, success := this.c.Verifyfunc(ctx, p.getpeeruniquename(), peerverifydata)
	if !success {
		log.Error("[Stream.verifypeer]", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename(), "verify failed")
		p.clientname = ""
		p.servername = ""
		return nil
	}
	return response
}
func (this *Instance) read(p *Peer) {
	defer func() {
		//every connection will have two goruntine to work for it
		if old := atomic.SwapUint32(&p.status, 0); old > 0 {
			//cause write goruntine return,this will be useful when there is nothing in writebuffer
			p.closeconn()
			//prevent write goruntine block on read channel
			p.writerbuffer <- (*bufpool.Buffer)(nil)
			p.heartbeatbuffer <- (*bufpool.Buffer)(nil)
		} else {
			if this.c.Offlinefunc != nil {
				this.c.Offlinefunc(p, p.getpeeruniquename(), p.starttime)
			}
			//when second goruntine return,put connection back to the pool
			this.noticech <- p
		}
	}()
	//after verify,the read timeout is useless,heartbeat will work for this
	switch p.protocol {
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
				if e == nil && msgtype != HEART && msgtype != USER && msgtype != CLOSEREAD && msgtype != CLOSEWRITE {
					e = errors.New("unknown msg type")
				}
			}
		}
		if e != nil {
			log.Error("[Stream.read] from", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename(), "error:", e)
			if data != nil {
				bufpool.PutBuffer(data)
			}
			return
		}
		//deal message
		switch msgtype {
		case HEART:
			//update lastactive time
			p.lastactive = uint64(time.Now().UnixNano())
		case USER:
			if !p.closewrite {
				userdata, starttime, _ := getUserMsg(data.Bytes())
				if starttime == p.starttime {
					//update lastactive time
					p.lastactive = uint64(time.Now().UnixNano())
					p.recvidlestart = p.lastactive
					this.c.Userdatafunc(p, p.getpeeruniquename(), userdata, starttime)
				}
				//drop race data
			}
		case CLOSEREAD:
			starttime, _ := getCloseReadMsg(data.Bytes())
			if starttime == p.starttime {
				p.closeread = true
				if p.closewrite {
					return
				}
				p.writerbuffer <- makeCloseWriteMsg(p.starttime, true)
			}
			//drop race data
		case CLOSEWRITE:
			starttime, _ := getCloseWriteMsg(data.Bytes())
			if starttime == p.starttime {
				p.closewrite = true
				if p.closeread {
					return
				}
				p.writerbuffer <- makeCloseReadMsg(p.starttime, true)
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
		for len(p.heartbeatbuffer) > 0 {
			if v := <-p.heartbeatbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		//every connection will have two goruntine to work for it
		if old := atomic.SwapUint32(&p.status, 0); old > 0 {
			//when first goruntine return,close the connection,cause read goruntine return
			p.closeconn()
		} else {
			if this.c.Offlinefunc != nil {
				this.c.Offlinefunc(p, p.getpeeruniquename(), p.starttime)
			}
			//when second goruntine return,put connection back to the pool
			this.noticech <- p
		}
	}()
	var data *bufpool.Buffer
	var ok bool
	var send int
	var num int
	var e error
	for {
		status := atomic.LoadUint32(&p.status)
		if (status == 0 || status == 2) && len(p.writerbuffer) == 0 {
			return
		}
		if len(p.heartbeatbuffer) > 0 {
			data, ok = <-p.heartbeatbuffer
		} else {
			select {
			case data, ok = <-p.writerbuffer:
			case data, ok = <-p.heartbeatbuffer:
			}
		}
		if p.closeread && p.closewrite {
			if data != nil {
				bufpool.PutBuffer(data)
			}
			return
		}
		if !ok || data == nil {
			continue
		}
		if !p.closeread {
			p.sendidlestart = uint64(time.Now().UnixNano())
			send = 0
			for send < data.Len() {
				switch p.protocol {
				case TCP:
					num, e = (*net.TCPConn)(p.conn).Write(data.Bytes()[send:])
				case UNIX:
					num, e = (*net.UnixConn)(p.conn).Write(data.Bytes()[send:])
				}
				if e != nil {
					log.Error("[Stream.write] to", p.protocol.protoname(), p.peertype.typename()+":", p.getpeeruniquename(), "error:", e)
					return
				}
				send += num
			}
		}
		bufpool.PutBuffer(data)
	}
}
