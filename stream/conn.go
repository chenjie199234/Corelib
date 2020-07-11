package stream

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	//"net/http"
	"syscall"
	"time"
	"unsafe"

	//"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func (this *Instance) StartTcpServer(verifydata []byte, listenaddr string) {
	var laddr *net.TCPAddr
	var l *net.TCPListener
	var e error
	var conn *net.TCPConn
	if laddr, e = net.ResolveTCPAddr("tcp", listenaddr); e != nil {
		panic("[Stream.TCP.StartTcpServer]resolve self addr error:" + e.Error())
	}
	if l, e = net.ListenTCP(laddr.Network(), laddr); e != nil {
		panic("[Stream.TCP.StartTcpServer]listening self addr error:" + e.Error())
	}
	go func() {
		for {
			p := this.getPeer()
			p.protocoltype = TCP
			p.servername = this.conf.SelfName
			p.peertype = CLIENT
			if conn, e = l.AcceptTCP(); e != nil {
				fmt.Printf("[Stream.TCP.StartTcpServer]accept tcp connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(this.conf.TcpSocketReadBufferLen, this.conf.TcpSocketWriteBufferLen)
			p.status = true
			go this.sworker(p, verifydata)
		}
	}()
}
func (this *Instance) StartUnixsocketServer(verifydata []byte, listenaddr string) {
	var laddr *net.UnixAddr
	var l *net.UnixListener
	var e error
	var conn *net.UnixConn
	if laddr, e = net.ResolveUnixAddr("unix", listenaddr); e != nil {
		panic("[Stream.UNIX.StartUnixsocketServer]resolve self addr error:" + e.Error())
	}
	if l, e = net.ListenUnix("unix", laddr); e != nil {
		panic("[Stream.UNIX.StartUnixsocketServer]listening self addr error:" + e.Error())
	}
	go func() {
		for {
			p := this.getPeer()
			p.protocoltype = UNIXSOCKET
			p.servername = this.conf.SelfName
			p.peertype = CLIENT
			if conn, e = l.AcceptUnix(); e != nil {
				fmt.Printf("[Stream.UNIX.StartUnixsocketServer]accept unix connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(this.conf.UnixSocketReadBufferLen, this.conf.UnixSocketWriteBufferLen)
			p.status = true
			go this.sworker(p, verifydata)
		}
	}()
}

//func (this *Instance) StartWebsocketServer(selfname string, verifydata []byte, listenaddr string) {
//}
func (this *Instance) sworker(p *Peer, verifydata []byte) bool {
	//read first verify message from client
	this.verifypeer(p, verifydata)
	if p.clientname != "" {
		//verify client success,send self's verify message to client
		verifymsg := makeVerifyMsg(p.servername, verifydata, p.starttime, true)
		send := 0
		num := 0
		var e error
		for send < len(verifymsg) {
			switch p.protocoltype {
			case TCP:
				num, e = (*net.TCPConn)(p.conn).Write(verifymsg[send:])
			case UNIXSOCKET:
				num, e = (*net.UnixConn)(p.conn).Write(verifymsg[send:])
			}
			switch {
			case e == nil: //don't print log when there is no error
			case e == syscall.EINVAL: //don't print log when conn is already closed
			default:
				if operr, ok := e.(*net.OpError); ok && operr != nil {
					if syserr, ok := operr.Err.(*os.SyscallError); ok && syserr != nil {
						if syserr.Err.(syscall.Errno) == syscall.ECONNRESET ||
							syserr.Err.(syscall.Errno) == syscall.EPIPE ||
							syserr.Err.(syscall.Errno) == syscall.EBADFD {
							//don't print log when err is rst
							//don't print log when err is broken pipe
							//don't print log when err is badfd
							break
						}
					}
				}
				switch p.protocoltype {
				case TCP:
					fmt.Printf("[Stream.TCP.sworker]write first verify msg error:%s to client addr:%s\n",
						e, (*net.TCPConn)(p.conn).RemoteAddr().String())
				case UNIXSOCKET:
					fmt.Printf("[Stream.UNIX.sworker]write first verify msg error:%s to client addr:%s\n",
						e, (*net.UnixConn)(p.conn).RemoteAddr().String())
				}
			}
			if e != nil {
				p.closeconn()
				this.putPeer(p)
				return false
			}
			send += num
		}
		if !this.addPeer(p) {
			switch p.protocoltype {
			case TCP:
				fmt.Printf("[Stream.TCP.sworker] refuse reconnect from client:%s addr:%s\n",
					p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
			case UNIXSOCKET:
				fmt.Printf("[Stream.UNIX.sworker] refuse reconnect from client:%s addr:%s\n",
					p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
			}
			return false
		}
		if this.onlinefunc != nil {
			this.onlinefunc(p, p.clientname, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		p.closeconn()
		this.putPeer(p)
		return false
	}
}

func (this *Instance) StartTcpClient(verifydata []byte, serveraddr string) bool {
	c, e := net.DialTimeout("tcp", serveraddr, time.Second)
	if e != nil {
		fmt.Printf("[Stream.TCP.StartTcpClient]tcp connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getPeer()
	p.protocoltype = TCP
	p.clientname = this.conf.SelfName
	p.peertype = SERVER
	p.conn = unsafe.Pointer(c.(*net.TCPConn))
	p.setbuffer(this.conf.TcpSocketReadBufferLen, this.conf.TcpSocketWriteBufferLen)
	p.status = true
	return this.cworker(p, verifydata)
}

func (this *Instance) StartUnixsocketClient(verifydata []byte, serveraddr string) bool {
	c, e := net.DialTimeout("unix", serveraddr, time.Second)
	if e != nil {
		fmt.Printf("[Stream.UNIX.StartUnixsocketClient]unix connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getPeer()
	p.protocoltype = UNIXSOCKET
	p.clientname = this.conf.SelfName
	p.peertype = SERVER
	p.conn = unsafe.Pointer(c.(*net.UnixConn))
	p.setbuffer(this.conf.UnixSocketReadBufferLen, this.conf.UnixSocketWriteBufferLen)
	p.status = true
	return this.cworker(p, verifydata)
}

//func (this *Instance) StartWebsocketClient(selfname string, verifydata []byte, serveraddr string) bool {

//}
func (this *Instance) cworker(p *Peer, verifydata []byte) bool {
	//send self's verify message to server
	verifymsg := makeVerifyMsg(p.clientname, verifydata, p.starttime, true)
	send := 0
	num := 0
	var e error
	for send < len(verifymsg) {
		switch p.protocoltype {
		case TCP:
			num, e = (*net.TCPConn)(p.conn).Write(verifymsg[send:])
		case UNIXSOCKET:
			num, e = (*net.UnixConn)(p.conn).Write(verifymsg[send:])
		}
		switch {
		case e == nil: //don't print log when there is no error
		case e == syscall.EINVAL: //don't print log when conn is already closed
		default:
			if operr, ok := e.(*net.OpError); ok && operr != nil {
				if syserr, ok := operr.Err.(*os.SyscallError); ok && syserr != nil {
					if syserr.Err.(syscall.Errno) == syscall.ECONNRESET ||
						syserr.Err.(syscall.Errno) == syscall.EPIPE ||
						syserr.Err.(syscall.Errno) == syscall.EBADFD {
						//don't print log when err is rst
						//don't print log when err is broken pipe
						//don't print log when err is badfd
						break
					}
				}
			}
			switch p.protocoltype {
			case TCP:
				fmt.Printf("[Stream.TCP.cworker]write first verify msg error:%s to server addr:%s\n",
					e, (*net.TCPConn)(p.conn).RemoteAddr().String())
			case UNIXSOCKET:
				fmt.Printf("[Stream.UNIX.cworker]write first verify msg error:%s to server addr:%s\n",
					e, (*net.UnixConn)(p.conn).RemoteAddr().String())
			}
		}
		if e != nil {
			p.closeconn()
			this.putPeer(p)
			return false
		}
		send += num
	}
	//read first verify message from server
	this.verifypeer(p, verifydata)
	if p.servername != "" {
		//verify server success
		if !this.addPeer(p) {
			switch p.protocoltype {
			case TCP:
				fmt.Printf("[Stream.TCP.cworker]refuse reconnect from server:%s addr:%s\n",
					p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
			case UNIXSOCKET:
				fmt.Printf("[Stream.UNIX.cworker]refuse reconnect from server:%s addr:%s\n",
					p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
			}
			return false
		}
		if this.onlinefunc != nil {
			this.onlinefunc(p, p.servername, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		this.putPeer(p)
		return false
	}
}
func (this *Instance) verifypeer(p *Peer, verifydata []byte) {
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	case UNIXSOCKET:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	}
	var e error
	for {
		if p.readbuffer.Rest() >= len(p.tempbuffer) {
			switch p.protocoltype {
			case TCP:
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer)
			case UNIXSOCKET:
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer)
			}
		} else {
			switch p.protocoltype {
			case TCP:
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			case UNIXSOCKET:
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		}
		switch {
		case e == nil: //don't print log when there is no error
		case e == io.EOF: //don't print log when err is eof
		case e == syscall.EINVAL: //don't print log when conn is already closed
		default:
			if operr, ok := e.(*net.OpError); ok && operr != nil {
				if syserr, ok := operr.Err.(*os.SyscallError); ok && syserr != nil {
					if syserr.Err.(syscall.Errno) == syscall.ECONNRESET ||
						syserr.Err.(syscall.Errno) == syscall.EPIPE ||
						syserr.Err.(syscall.Errno) == syscall.EBADFD {
						//don't print log when err is rst
						//don't print log when err is broken pipe
						//don't print log when err is badfd
						break
					}
				}
			}
			switch p.peertype {
			case CLIENT:
				switch p.protocoltype {
				case TCP:
					fmt.Printf("[Stream.TCP.sworker]read first verify msg error:%s from client addr:%s\n",
						e, (*net.TCPConn)(p.conn).RemoteAddr().String())
				case UNIXSOCKET:
					fmt.Printf("[Stream.UNIX.sworker]read first verify msg error:%s from client addr:%s\n",
						e, (*net.UnixConn)(p.conn).RemoteAddr().String())
				}
			case SERVER:
				switch p.protocoltype {
				case TCP:
					fmt.Printf("[Stream.TCP.cworker]read first verify msg error:%s from server addr:%s\n",
						e, (*net.TCPConn)(p.conn).RemoteAddr().String())
				case UNIXSOCKET:
					fmt.Printf("[Stream.UNIX.cworker]read first verify msg error:%s from server addr:%s\n",
						e, (*net.UnixConn)(p.conn).RemoteAddr().String())
				}
			}
		}
		if e != nil {
			return
		}
		p.readbuffer.Put(p.tempbuffer[:p.tempbuffernum])
		for {
			if p.readbuffer.Num() <= 2 {
				break
			}
			msglen := binary.BigEndian.Uint16(p.readbuffer.Peek(0, 2))
			if p.readbuffer.Num() < int(msglen+2) {
				if p.readbuffer.Rest() == 0 {
					switch p.peertype {
					case CLIENT:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.sworker]first verify msg too long from client addr:%s\n",
								(*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.sworker]first verify msg too long from client addr:%s\n",
								(*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					case SERVER:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.cworker]first verify msg too long from server addr:%s\n",
								(*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.cworker]first verify msg too long from server addr:%s\n",
								(*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					}
					return
				}
				break
			}
			msg := &TotalMsg{}
			if e := proto.Unmarshal(p.readbuffer.Get(int(msglen + 2))[2:], msg); e != nil {
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.sworker]first verify msg format error:%s from client addr:%s\n",
							e, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.sworker]first veridy msg format error:%s from client addr:%s\n",
							e, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.cworker]first verify msg format error:%s from server addr:%s\n",
							e, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.cworker]first verify msg format error:%s from server addr:%s\n",
							e, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				}
				return
			}
			//first message must be verify message
			if msg.Totaltype != TotalMsgType_VERIFY {
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.sworker]first msg isn't verify msg from client addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.sworker]first msg isn't verify msg from client addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.cworker]first msg isn't verify msg from server addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.cworker]first msg isn't verify msg from server addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				}
				return
			}
			switch p.peertype {
			case CLIENT:
				if msg.Sender == "" || msg.Sender == p.servername {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.sworker]sender name check failed from client addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.sworker]sender name check failed from client addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
				if msg.GetVerify() == nil {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.sworker]verify data is empty from client addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.sworker]verify data is empty from client addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
				if !this.verifyfunc(p.servername, verifydata, msg.Sender, msg.GetVerify().Verifydata) {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.sworker]verify failed with data:%s from client addr:%s\n",
							msg.GetVerify().Verifydata, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.sworker]verify failed with data:%s from client addr:%s\n",
							msg.GetVerify().Verifydata, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
			case SERVER:
				if msg.Sender == "" || msg.Sender == p.clientname {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.cworker]sender name check failed from server addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.cworker]sender name check failed from server addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
				if msg.GetVerify() == nil {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.cworker]verify data is empty from server addr:%s\n",
							(*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.cworker]verify data is empty from server addr:%s\n",
							(*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
				if !this.verifyfunc(p.clientname, verifydata, msg.Sender, msg.GetVerify().Verifydata) {
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.cworker]verify failed with data:%s from server addr:%s\n",
							msg.GetVerify().Verifydata, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.cworker]verify failed with data:%s from server addr:%s\n",
							msg.GetVerify().Verifydata, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
					return
				}
			}
			p.lastactive = time.Now().UnixNano()
			switch p.peertype {
			case CLIENT:
				p.starttime = p.lastactive
				p.clientname = msg.Sender
			case SERVER:
				p.starttime = msg.Starttime
				p.servername = msg.Sender
			}
			return
		}
	}
}
func (this *Instance) read(p *Peer) {
	defer func() {
		p.parentnode.Lock()
		var ok bool
		switch p.peertype {
		case CLIENT:
			_, ok = p.parentnode.peers[p.clientname]
		case SERVER:
			_, ok = p.parentnode.peers[p.servername]
		}
		//every connection will have two goruntine to work for it
		if ok {
			//when first goruntine return,delete this connection from the map
			switch p.peertype {
			case CLIENT:
				delete(p.parentnode.peers, p.clientname)
			case SERVER:
				delete(p.parentnode.peers, p.servername)
			}
			//cause write goruntine return,this will be useful when there is nothing in writebuffer
			p.status = false
			p.writerbuffer <- []byte{}
			p.parentnode.Unlock()
		} else {
			p.parentnode.Unlock()
			//when second goruntine return,put connection back to the pool
			if this.offlinefunc != nil {
				switch p.peertype {
				case CLIENT:
					this.offlinefunc(p, p.clientname, p.starttime)
				case SERVER:
					this.offlinefunc(p, p.servername, p.starttime)
				}
			}
			this.putPeer(p)
		}
	}()
	//after verify,the read timeout is useless,heartbeat will work for this
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Time{})
	case UNIXSOCKET:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Time{})
	}
	var e error
	for {
		if p.readbuffer.Rest() >= len(p.tempbuffer) {
			switch p.protocoltype {
			case TCP:
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer)
			case UNIXSOCKET:
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer)
			}
		} else {
			switch p.protocoltype {
			case TCP:
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			case UNIXSOCKET:
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		}
		switch {
		case e == nil: //don't print log when there is no error
		case e == io.EOF: //don't print log when err is eof
		case e == syscall.EINVAL: //don't print log when conn is already closed
		default:
			if operr, ok := e.(*net.OpError); ok && operr != nil {
				if syserr, ok := operr.Err.(*os.SyscallError); ok && syserr != nil {
					if syserr.Err.(syscall.Errno) == syscall.ECONNRESET ||
						syserr.Err.(syscall.Errno) == syscall.EPIPE ||
						syserr.Err.(syscall.Errno) == syscall.EBADFD {
						//don't print log when err is rst
						//don't print log when err is broken pipe
						//don't print log when err is badfd
						break
					}
				}
			}
			switch p.peertype {
			case CLIENT:
				switch p.protocoltype {
				case TCP:
					fmt.Printf("[Stream.TCP.read]read msg error:%s from client:%s addr:%s\n",
						e, p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
				case UNIXSOCKET:
					fmt.Printf("[Stream.UNIX.read]read msg error:%s from client:%s addr:%s\n",
						e, p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
				}
			case SERVER:
				switch p.protocoltype {
				case TCP:
					fmt.Printf("[Stream.TCP.read]read msg error:%s from server:%s addr:%s\n",
						e, p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
				case UNIXSOCKET:
					fmt.Printf("[Stream.UNIX.read]read msg error:%s from server:%s addr:%s\n",
						e, p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
				}
			}
		}
		if e != nil {
			return
		}
		p.readbuffer.Put(p.tempbuffer[:p.tempbuffernum])
		for {
			if p.readbuffer.Num() <= 2 {
				break
			}
			msglen := binary.BigEndian.Uint16(p.readbuffer.Peek(0, 2))
			if p.readbuffer.Num() < int(msglen+2) {
				if p.readbuffer.Rest() == 0 {
					switch p.peertype {
					case CLIENT:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.read]msg too long from client:%s addr:%s\n",
								p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.read]msg too long from client:%s addr:%s\n",
								p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					case SERVER:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.read]msg too long from server:%s addr:%s\n",
								p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.read]msg too long from server:%s addr:%s\n",
								p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
						}
					}
					return
				}
				break
			}
			msg := &TotalMsg{}
			data := p.readbuffer.Get(int(msglen + 2))
			if e = proto.Unmarshal(data[2:], msg); e != nil {
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.read]msg format error:%s from client:%s addr:%s\n",
							e, p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.read]msg format error:%s from client:%s addr:%s\n",
							e, p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.read]msg format error:%s from server:%s addr:%s\n",
							e, p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.read]msg format error:%s from server:%s addr:%s\n",
							e, p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				}
				return
			}
			//drop data race message
			if msg.Starttime != p.starttime {
				continue
			}
			//deal message
			switch msg.Totaltype {
			case TotalMsgType_HEART:
				//update lastactive time
				p.lastactive = time.Now().UnixNano()
				switch p.peertype {
				case CLIENT:
					switch msg.Sender {
					case p.clientname:
						//sendback
						p.writerbuffer <- data
					case p.servername:
						if msg.GetHeart() == nil {
							switch p.protocoltype {
							case TCP:
								fmt.Printf("[Stream.TCP.read]self heart msg empty return from client:%s addr:%s\n",
									p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
							case UNIXSOCKET:
								fmt.Printf("[Stream.UNIX.read]self heart msg empty return from client:%s addr:%s\n",
									p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
							}
							return
						}
						if msg.GetHeart().Timestamp > p.lastactive {
							switch p.protocoltype {
							case TCP:
								fmt.Printf("[Stream.TCP.read]self heart msg time check error return from client:%s addr:%s",
									p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
							case UNIXSOCKET:
								fmt.Printf("[Stream.TCP.read]self heart msg time check error return from client:%s addr:%s",
									p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
							}
							return
						}
						p.netlag[p.netlagindex] = p.lastactive - msg.GetHeart().Timestamp
						p.netlagindex++
						if p.netlagindex >= len(p.netlag) {
							p.netlagindex = 0
						}
					default:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.read]heart msg sender name:%s check failed from client:%s addr:%s selfname:%s\n",
								msg.Sender, p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String(), p.servername)
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.read]heart msg sender name:%s check failed from client:%s addr:%s selfname:%s\n",
								msg.Sender, p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String(), p.servername)
						}
						return
					}
				case SERVER:
					switch msg.Sender {
					case p.clientname:
						if msg.GetHeart() == nil {
							switch p.protocoltype {
							case TCP:
								fmt.Printf("[Stream.TCP.read]self heart msg empty return from server:%s addr:%s\n",
									p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
							case UNIXSOCKET:
								fmt.Printf("[Stream.UNIX.read]self heart msg empty return from server:%s addr:%s\n",
									p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
							}
							return
						}
						if msg.GetHeart().Timestamp > p.lastactive {
							switch p.protocoltype {
							case TCP:
								fmt.Printf("[Stream.TCP.read]self heart msg time check error return from server:%s addr:%s",
									p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
							case UNIXSOCKET:
								fmt.Printf("[Stream.TCP.read]self heart msg time check error return from server:%s addr:%s",
									p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
							}
							return
						}
						p.netlag[p.netlagindex] = p.lastactive - msg.GetHeart().Timestamp
						p.netlagindex++
						if p.netlagindex >= len(p.netlag) {
							p.netlagindex = 0
						}
					case p.servername:
						//sendback
						p.writerbuffer <- data
					default:
						switch p.protocoltype {
						case TCP:
							fmt.Printf("[Stream.TCP.read]heart msg sender name:%s check failed from server:%s addr:%s selfname:%s\n",
								msg.Sender, p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String(), p.clientname)
						case UNIXSOCKET:
							fmt.Printf("[Stream.UNIX.read]heart msg sender name:%s check failed from server:%s addr:%s selfname:%s\n",
								msg.Sender, p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String(), p.clientname)
						}
						return
					}
				}
			case TotalMsgType_USER:
				p.lastactive = time.Now().UnixNano()
				switch p.peertype {
				case CLIENT:
					this.userdatafunc(p, p.clientname, p.starttime, msg.GetUser().Userdata)
				case SERVER:
					this.userdatafunc(p, p.servername, p.starttime, msg.GetUser().Userdata)
				}
			}
		}
	}
}
func (this *Instance) write(p *Peer) {
	defer func() {
		//drop all data
		for len(p.writerbuffer) > 0 {
			<-p.writerbuffer
		}
		p.parentnode.Lock()
		var ok bool
		switch p.peertype {
		case CLIENT:
			_, ok = p.parentnode.peers[p.clientname]
		case SERVER:
			_, ok = p.parentnode.peers[p.servername]
		}
		//every connection will have two goruntine to work for it
		if ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.clientname+p.servername)
			//close the connection,cause read goruntine return
			p.closeconn()
			p.status = false
			p.parentnode.Unlock()
		} else {
			p.parentnode.Unlock()
			//when second goruntine return,put connection back to the pool
			if this.offlinefunc != nil {
				switch p.peertype {
				case CLIENT:
					this.offlinefunc(p, p.clientname, p.starttime)
				case SERVER:
					this.offlinefunc(p, p.servername, p.starttime)
				}
			}
			this.putPeer(p)
		}
	}()
	send := 0
	num := 0
	var e error
	for {
		data, ok := <-p.writerbuffer
		if !ok || len(data) == 0 {
			return
		}
		send = 0
		for send < len(data) {
			switch p.protocoltype {
			case TCP:
				num, e = (*net.TCPConn)(p.conn).Write(data[send:])
			case UNIXSOCKET:
				num, e = (*net.UnixConn)(p.conn).Write(data[send:])
			}
			switch {
			case e == nil: //don't print log when there is no error
			case e == syscall.EINVAL: //don't print log when conn is already closed
			default:
				if operr, ok := e.(*net.OpError); ok && operr != nil {
					if syserr, ok := operr.Err.(*os.SyscallError); ok && syserr != nil {
						if syserr.Err.(syscall.Errno) == syscall.ECONNRESET ||
							syserr.Err.(syscall.Errno) == syscall.EPIPE ||
							syserr.Err.(syscall.Errno) == syscall.EBADFD {
							//don't print log when err is rst
							//don't print log when err is broken pipe
							//don't print log when err is badfd
							break
						}
					}
				}
				switch p.peertype {
				case CLIENT:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.write]write msg error:%s to client:%s addr:%s\n",
							e, p.clientname, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.write]write msg error:%s to client:%s addr:%s\n",
							e, p.clientname, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				case SERVER:
					switch p.protocoltype {
					case TCP:
						fmt.Printf("[Stream.TCP.write]write msg error:%s to servername:%s addr:%s\n",
							e, p.servername, (*net.TCPConn)(p.conn).RemoteAddr().String())
					case UNIXSOCKET:
						fmt.Printf("[Stream.UNIX.write]write msg error:%s to servername:%s addr:%s\n",
							e, p.servername, (*net.UnixConn)(p.conn).RemoteAddr().String())
					}
				}
			}
			if e != nil {
				return
			}
			send += num
		}
	}
}
