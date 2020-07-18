package stream

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func (this *Instance) StartTcpServer(c *TcpConfig, listenaddr string) {
	if c == nil {
		c = &TcpConfig{}
	}
	checkTcpConfig(c)
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
			p := this.getPeer(TCP, unsafe.Pointer(c))
			p.protocoltype = TCP
			p.servername = this.conf.SelfName
			p.peertype = CLIENT
			if conn, e = l.AcceptTCP(); e != nil {
				fmt.Printf("[Stream.TCP.StartTcpServer]accept tcp connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
			p.status = true
			go this.sworker(p)
		}
	}()
}
func (this *Instance) StartUnixsocketServer(c *UnixConfig, listenaddr string) {
	if c == nil {
		c = &UnixConfig{}
	}
	checkUnixConfig(c)
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
			p := this.getPeer(UNIXSOCKET, unsafe.Pointer(c))
			p.protocoltype = UNIXSOCKET
			p.servername = this.conf.SelfName
			p.peertype = CLIENT
			if conn, e = l.AcceptUnix(); e != nil {
				fmt.Printf("[Stream.UNIX.StartUnixsocketServer]accept unix connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
			p.status = true
			go this.sworker(p)
		}
	}()
}

func (this *Instance) StartWebsocketServer(c *WebConfig, paths []string, listenaddr string, checkorigin func(*http.Request) bool) {
	if c == nil {
		c = &WebConfig{}
	}
	checkWebConfig(c)
	upgrader := &websocket.Upgrader{
		HandshakeTimeout:  time.Duration(c.ConnectTimeout) * time.Millisecond,
		WriteBufferPool:   &sync.Pool{},
		CheckOrigin:       checkorigin,
		EnableCompression: c.EnableCompress,
	}
	server := &http.Server{
		Addr:              listenaddr,
		ReadHeaderTimeout: time.Duration(c.ConnectTimeout) * time.Millisecond,
		ReadTimeout:       time.Duration(c.ConnectTimeout) * time.Millisecond,
		MaxHeaderBytes:    c.HttpMaxHeaderLen,
		Handler:           mux.NewRouter(),
	}
	connhandler := func(w http.ResponseWriter, r *http.Request) {
		conn, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			fmt.Printf("[Stream.WEB.StartWebsocketServer]upgrade error:%s\n", e)
			return
		}
		p := this.getPeer(WEBSOCKET, unsafe.Pointer(c))
		p.protocoltype = WEBSOCKET
		p.servername = this.conf.SelfName
		p.peertype = CLIENT
		p.conn = unsafe.Pointer(conn)
		p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
		p.status = true
		go this.sworker(p)
	}
	for _, path := range paths {
		(server.Handler).(*mux.Router).HandleFunc(path, connhandler)
	}
	if c.TlsCertFile != "" && c.TlsKeyFile != "" {
		go func() {
			if e := server.ListenAndServeTLS(c.TlsCertFile, c.TlsKeyFile); e != nil {
				panic("[Stream.WEB.StartWebsocketServer]start wss server error:" + e.Error())
			}
		}()
	} else {
		go func() {
			if e := server.ListenAndServe(); e != nil {
				panic("[Stream.WEB.StartWebsocketServer]start ws server error:" + e.Error())
			}
		}()
	}
}
func (this *Instance) sworker(p *Peer) bool {
	//read first verify message from client
	this.verifypeer(p)
	if p.clientname != "" {
		//verify client success,send self's verify message to client
		var verifymsg []byte
		switch p.protocoltype {
		case TCP:
			fallthrough
		case UNIXSOCKET:
			verifymsg = makeVerifyMsg(p.servername, this.conf.VerifyData, p.starttime, true)
		case WEBSOCKET:
			verifymsg = makeVerifyMsg(p.servername, this.conf.VerifyData, p.starttime, false)
		}
		send := 0
		num := 0
		var e error
		for send < len(verifymsg) {
			switch p.protocoltype {
			case TCP:
				num, e = (*net.TCPConn)(p.conn).Write(verifymsg[send:])
			case UNIXSOCKET:
				num, e = (*net.UnixConn)(p.conn).Write(verifymsg[send:])
			case WEBSOCKET:
				e = (*websocket.Conn)(p.conn).WriteMessage(websocket.BinaryMessage, verifymsg)
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
				fmt.Printf("[Stream.%s.sworker]write first verify msg error:%s to client addr:%s\n",
					p.getprotocolname(), e, p.getpeeraddr())
			}
			if e != nil {
				p.closeconn()
				p.status = false
				this.putPeer(p)
				return false
			}
			switch p.protocoltype {
			case TCP:
				fallthrough
			case UNIXSOCKET:
				send += num
			case WEBSOCKET:
				send = len(verifymsg)

			}
		}
		if !this.addPeer(p) {
			fmt.Printf("[Stream.%s.sworker]refuse reconnect from client:%s addr:%s\n",
				p.getprotocolname(), p.clientname, p.getpeeraddr())
			return false
		}
		//set websocket pong handler
		if p.protocoltype == WEBSOCKET {
			(*websocket.Conn)(p.conn).SetPongHandler(func(data string) error {
				return this.dealmsg(p, str2byte(data), true)
			})
		}
		if this.conf.Onlinefunc != nil {
			this.conf.Onlinefunc(p, p.clientname, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		p.closeconn()
		p.status = false
		this.putPeer(p)
		return false
	}
}

func (this *Instance) StartTcpClient(c *TcpConfig, serveraddr string) bool {
	if c == nil {
		c = &TcpConfig{}
	}
	checkTcpConfig(c)
	conn, e := net.DialTimeout("tcp", serveraddr, time.Duration(c.ConnectTimeout)*time.Millisecond)
	if e != nil {
		fmt.Printf("[Stream.TCP.StartTcpClient]tcp connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getPeer(TCP, unsafe.Pointer(c))
	p.protocoltype = TCP
	p.clientname = this.conf.SelfName
	p.peertype = SERVER
	p.conn = unsafe.Pointer(conn.(*net.TCPConn))
	p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
	p.status = true
	return this.cworker(p)
}

func (this *Instance) StartUnixsocketClient(c *UnixConfig, serveraddr string) bool {
	if c == nil {
		c = &UnixConfig{}
	}
	checkUnixConfig(c)
	conn, e := net.DialTimeout("unix", serveraddr, time.Duration(c.ConnectTimeout)*time.Millisecond)
	if e != nil {
		fmt.Printf("[Stream.UNIX.StartUnixsocketClient]unix connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getPeer(UNIXSOCKET, unsafe.Pointer(c))
	p.protocoltype = UNIXSOCKET
	p.clientname = this.conf.SelfName
	p.peertype = SERVER
	p.conn = unsafe.Pointer(conn.(*net.UnixConn))
	p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
	p.status = true
	return this.cworker(p)
}

func (this *Instance) StartWebsocketClient(c *WebConfig, serveraddr string) bool {
	dialer := &websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, time.Duration(c.ConnectTimeout)*time.Millisecond)
		},
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  time.Duration(c.ConnectTimeout) * time.Millisecond,
		WriteBufferPool:   &sync.Pool{},
		EnableCompression: c.EnableCompress,
	}
	conn, _, e := dialer.Dial(serveraddr, nil)
	if e != nil {
		fmt.Printf("[Stream.WEB.StartWebsocketClient]websocket connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getPeer(WEBSOCKET, unsafe.Pointer(c))
	p.protocoltype = WEBSOCKET
	p.clientname = this.conf.SelfName
	p.peertype = SERVER
	p.conn = unsafe.Pointer(conn)
	p.setbuffer(c.SocketReadBufferLen, c.SocketWriteBufferLen)
	p.status = true
	return this.cworker(p)
}

func (this *Instance) cworker(p *Peer) bool {
	//send self's verify message to server
	var verifymsg []byte
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		verifymsg = makeVerifyMsg(p.clientname, this.conf.VerifyData, p.starttime, true)
	case WEBSOCKET:
		verifymsg = makeVerifyMsg(p.clientname, this.conf.VerifyData, p.starttime, false)
	}
	send := 0
	num := 0
	var e error
	for send < len(verifymsg) {
		switch p.protocoltype {
		case TCP:
			num, e = (*net.TCPConn)(p.conn).Write(verifymsg[send:])
		case UNIXSOCKET:
			num, e = (*net.UnixConn)(p.conn).Write(verifymsg[send:])
		case WEBSOCKET:
			e = (*websocket.Conn)(p.conn).WriteMessage(websocket.BinaryMessage, verifymsg)
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
			fmt.Printf("[Stream.%s.cworker]write first verify msg error:%s to server addr:%s\n",
				p.getprotocolname(), e, p.getpeeraddr())
		}
		if e != nil {
			p.closeconn()
			this.putPeer(p)
			return false
		}
		switch p.protocoltype {
		case TCP:
			fallthrough
		case UNIXSOCKET:
			send += num
		case WEBSOCKET:
			send = len(verifymsg)
		}
	}
	//read first verify message from server
	this.verifypeer(p)
	if p.servername != "" {
		//verify server success
		if !this.addPeer(p) {
			fmt.Printf("[Stream.%s.cworker]refuse reconnect to server:%s addr:%s\n",
				p.getprotocolname(), p.getpeername(), p.getpeeraddr())
			return false
		}
		if p.protocoltype == WEBSOCKET {
			(*websocket.Conn)(p.conn).SetPongHandler(func(data string) error {
				return this.dealmsg(p, str2byte(data), true)
			})
		}
		if this.conf.Onlinefunc != nil {
			this.conf.Onlinefunc(p, p.servername, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		p.closeconn()
		p.status = false
		this.putPeer(p)
		return false
	}
}
func (this *Instance) verifypeer(p *Peer) {
	switch p.protocoltype {
	case TCP:
		(*net.TCPConn)(p.conn).SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	case UNIXSOCKET:
		(*net.UnixConn)(p.conn).SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	case WEBSOCKET:
		(*websocket.Conn)(p.conn).SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	}
	var e error
	var data []byte
	for {
		switch p.protocoltype {
		case TCP:
			if p.readbuffer.Rest() > len(p.tempbuffer) {
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer)
			} else {
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		case UNIXSOCKET:
			if p.readbuffer.Rest() > len(p.tempbuffer) {
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer)
			} else {
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		case WEBSOCKET:
			_, data, e = (*websocket.Conn)(p.conn).ReadMessage()
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
			fmt.Printf("[Stream.%s.verifypeer]read first verify msg error:%s from %s addr:%s\n",
				p.getprotocolname(), e, p.getpeertypename(), p.getpeeraddr())
		}
		if e != nil {
			return
		}
		if p.protocoltype != WEBSOCKET {
			p.readbuffer.Put(p.tempbuffer[:p.tempbuffernum])
			if p.readbuffer.Num() <= 2 {
				continue
			}
			msglen := binary.BigEndian.Uint16(p.readbuffer.Peek(0, 2))
			if p.readbuffer.Num() >= int(msglen+2) {
				data = p.readbuffer.Get(int(msglen + 2))[2:]
				break
			}
			if p.readbuffer.Rest() == 0 {
				fmt.Printf("[Stream.%s.verifypeer]first verify msg too long from %s addr:%s\n",
					p.getprotocolname(), p.getpeertypename(), p.getpeeraddr())
				return
			}
		} else {
			break
		}
	}
	msg := &TotalMsg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		fmt.Printf("[Stream.%s.verifypeer]first verify msg format error:%s from %s addr:%s\n",
			p.getprotocolname(), e, p.getpeertypename(), p.getpeeraddr())
		return
	}
	//first message must be verify message
	if msg.Totaltype != TotalMsgType_VERIFY {
		fmt.Printf("[Stream.%s.verifypeer]first msg isn't verify msg from %s addr:%s\n",
			p.getprotocolname(), p.getpeertypename(), p.getpeeraddr())
		return
	}
	if msg.Sender == "" || msg.Sender == p.getselfname() {
		fmt.Printf("[Stream.%s.verifypeer]sender name:%s check failed from %s addr:%s\n",
			p.getprotocolname(), msg.Sender, p.getpeertypename(), p.getpeeraddr())
		return
	}
	if msg.GetVerify() == nil {
		fmt.Printf("[Stream.%s.verifypeer]verify data is empty from %s addr:%s\n",
			p.getprotocolname(), p.getpeertypename(), p.getpeeraddr())
		return
	}
	if !this.conf.Verifyfunc(p.getselfname(), this.conf.VerifyData, msg.Sender, msg.GetVerify().Verifydata) {
		fmt.Printf("[Stream.%s.verifypeer]verify failed with data:%s from %s addr:%s\n",
			p.getprotocolname(), msg.GetVerify().Verifydata, p.getpeertypename(), p.getpeeraddr())
		return
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
func (this *Instance) read(p *Peer) {
	defer func() {
		//every connection will have two goruntine to work for it
		p.parentnode.Lock()
		if _, ok := p.parentnode.peers[p.getpeername()]; ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.getpeername())
			//cause write goruntine return,this will be useful when there is nothing in writebuffer
			p.status = false
			p.writerbuffer <- []byte{}
			p.heartbeatbuffer <- []byte{}
			p.parentnode.Unlock()
		} else {
			p.parentnode.Unlock()
			//when second goruntine return,put connection back to the pool
			if this.conf.Offlinefunc != nil {
				this.conf.Offlinefunc(p, p.getpeername(), p.starttime)
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
	case WEBSOCKET:
		(*websocket.Conn)(p.conn).UnderlyingConn().SetDeadline(time.Time{})
	}
	var e error
	data := []byte{}
	for {
		switch p.protocoltype {
		case TCP:
			if p.readbuffer.Rest() >= len(p.tempbuffer) {
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer)
			} else {
				p.tempbuffernum, e = (*net.TCPConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		case UNIXSOCKET:
			if p.readbuffer.Rest() >= len(p.tempbuffer) {
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer)
			} else {
				p.tempbuffernum, e = (*net.UnixConn)(p.conn).Read(p.tempbuffer[:p.readbuffer.Rest()])
			}
		case WEBSOCKET:
			_, data, e = (*websocket.Conn)(p.conn).ReadMessage()
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
			fmt.Printf("[Stream.%s.read]read msg error:%s from %s:%s addr:%s\n",
				p.getprotocolname(), e, p.getpeertypename(), p.getpeername(), p.getpeeraddr())
		}
		if e != nil {
			return
		}
		if p.protocoltype != WEBSOCKET {
			//tcp and unix socket
			p.readbuffer.Put(p.tempbuffer[:p.tempbuffernum])
			for {
				if p.readbuffer.Num() <= 2 {
					break
				}
				msglen := binary.BigEndian.Uint16(p.readbuffer.Peek(0, 2))
				if msglen == 0 {
					p.readbuffer.Get(2)
					continue
				}
				if p.readbuffer.Num() < int(msglen+2) {
					if p.readbuffer.Rest() == 0 {
						fmt.Printf("[Stream.%s.read]msg too long from %s:%s addr:%s\n",
							p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
						return
					}
					break
				}
				data = p.readbuffer.Get(int(msglen + 2))
				if e := this.dealmsg(p, data, false); e != nil {
					fmt.Println(e)
					return
				}
			}
		} else if len(data) != 0 {
			if e := this.dealmsg(p, data, false); e != nil {
				fmt.Println(e)
				return
			}
		}
	}
}
func (this *Instance) dealmsg(p *Peer, data []byte, frompong bool) error {
	var e error
	msg := &TotalMsg{}
	switch p.protocoltype {
	case TCP:
		fallthrough
	case UNIXSOCKET:
		if e = proto.Unmarshal(data[2:], msg); e != nil {
			return fmt.Errorf("[Stream.%s.dealmsg]msg format error:%s from %s:%s addr:%s",
				p.getprotocolname(), e, p.getpeertypename(), p.getpeername(), p.getpeeraddr())
		}
	case WEBSOCKET:
		if e = proto.Unmarshal(data, msg); e != nil {
			return fmt.Errorf("[Stream.WEB.dealmsg]msg format error:%s from %s:%s addr:%s",
				e, p.getpeertypename(), p.getpeername(), p.getpeeraddr())
		}
		if !frompong {
			if msg.Totaltype == TotalMsgType_HEART {
				return fmt.Errorf("[Stream.WEB.dealmsg]msg type error from %s:%s addr:%s",
					p.getpeertypename(), p.getpeername(), p.getpeeraddr())
			}
		} else {
			if msg.Totaltype != TotalMsgType_HEART {
				return fmt.Errorf("[Stream.WEB.dealmsg]msg type error from %s:%s addr:%s",
					p.getpeertypename(), p.getpeername(), p.getpeeraddr())
			}
			if msg.Sender != p.getselfname() {
				return fmt.Errorf("[Stream.WEB.dealmsg]pong msg sender:%s isn't self:%s from %s:%s addr:%s",
					msg.Sender, p.getselfname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
			}
		}
	}
	//drop data race message
	if msg.Starttime != p.starttime {
		return nil
	}
	//deal message
	switch msg.Totaltype {
	case TotalMsgType_HEART:
		return this.dealheart(p, msg, data)
	case TotalMsgType_USER:
		return this.dealuser(p, msg)
	default:
		return fmt.Errorf("[Stream.%s.dealmsg]get unknown type msg from %s:%s addr:%s",
			p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
	}
}
func (this *Instance) dealheart(p *Peer, msg *TotalMsg, data []byte) error {
	//update lastactive time
	p.lastactive = time.Now().UnixNano()
	switch msg.Sender {
	case p.getpeername():
		//send back
		p.heartbeatbuffer <- data
		return nil
	case p.getselfname():
		if msg.GetHeart() == nil {
			return fmt.Errorf("[Stream.%s.dealheart]self heart msg empty return from %s:%s addr:%s",
				p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
		}
		if msg.GetHeart().Timestamp > p.lastactive {
			return fmt.Errorf("[Stream.%s.dealheart]self heart msg time check error return from %s:%s addr:%s",
				p.getprotocolname(), p.getpeertypename(), p.getpeername(), p.getpeeraddr())
		}
		//update net lag
		p.netlag[p.netlagindex] = p.lastactive - msg.GetHeart().Timestamp
		p.netlagindex++
		if p.netlagindex >= len(p.netlag) {
			p.netlagindex = 0
		}
		return nil
	default:
		return fmt.Errorf("[Stream.%s.dealheart]heart msg sender name:%s check failed from %s:%s addr:%s selfname:%s",
			p.getprotocolname(), msg.Sender, p.getpeertypename(), p.getpeername(), p.getpeeraddr(), p.getselfname())
	}
}
func (this *Instance) dealuser(p *Peer, msg *TotalMsg) error {
	//update lastactive time
	p.lastactive = time.Now().UnixNano()
	this.conf.Userdatafunc(p, p.getpeername(), p.starttime, msg.GetUser().Userdata)
	return nil
}
func (this *Instance) write(p *Peer) {
	defer func() {
		//drop all data
		for len(p.writerbuffer) > 0 {
			<-p.writerbuffer
		}
		for len(p.heartbeatbuffer) > 0 {
			<-p.heartbeatbuffer
		}
		p.parentnode.Lock()
		//every connection will have two goruntine to work for it
		if _, ok := p.parentnode.peers[p.getpeername()]; ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.getpeername())
			//close the connection,cause read goruntine return
			p.closeconn()
			p.status = false
			p.parentnode.Unlock()
		} else {
			p.parentnode.Unlock()
			//when second goruntine return,put connection back to the pool
			if this.conf.Offlinefunc != nil {
				this.conf.Offlinefunc(p, p.getpeername(), p.starttime)
			}
			this.putPeer(p)
		}
	}()
	var data []byte
	var ok bool
	var send int
	var num int
	var e error
	var isheart bool
	for {
		isheart = false
		select {
		case data, ok = <-p.heartbeatbuffer:
			isheart = true
		default:
			select {
			case data, ok = <-p.writerbuffer:
			case data, ok = <-p.heartbeatbuffer:
				isheart = true
			}
		}
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
			case WEBSOCKET:
				if isheart {
					e = (*websocket.Conn)(p.conn).WriteMessage(websocket.PingMessage, data)
				} else {
					e = (*websocket.Conn)(p.conn).WriteMessage(websocket.BinaryMessage, data)
				}
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
				fmt.Printf("[Stream.%s.write]write msg error:%s to %s:%s addr:%s\n",
					p.getprotocolname(), e, p.getpeertypename(), p.getpeername(), p.getpeeraddr())
			}
			if e != nil {
				return
			}
			switch p.protocoltype {
			case TCP:
				fallthrough
			case UNIXSOCKET:
				send += num
			case WEBSOCKET:
				send = len(data)
			}
		}
	}
}