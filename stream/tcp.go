package stream

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
	"unsafe"

	"google.golang.org/protobuf/proto"
)

func (this *Instance) StartTcpServer(selfname string, verifydata []byte, listenaddr string) {
	var laddr *net.TCPAddr
	var l *net.TCPListener
	var e error
	var conn *net.TCPConn
	if laddr, e = net.ResolveTCPAddr("tcp", listenaddr); e != nil {
		panic("[Stream.StartTcpServer]resolve self addr error:%s" + e.Error())
	}
	if l, e = net.ListenTCP(laddr.Network(), laddr); e != nil {
		panic("[Stream.StartTcpServer]listening self addr error:%s" + e.Error())
	}
	go func() {
		for {
			p := this.getpeer()
			p.protocoltype = TCP
			p.servername = selfname
			p.selftype = SERVER
			if conn, e = l.AcceptTCP(); e != nil {
				fmt.Printf("[Stream.StartTcpServer]accept tcp connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(this.conf.MinReadBufferLen)
			p.status = true
			go this.sworker(p, verifydata)
		}
	}()
}
func (this *Instance) sworker(p *Peer, verifydata []byte) bool {
	//read first verify message from client
	this.verifypeer(p, verifydata)
	if p.clientname != "" {
		conn := (*net.TCPConn)(p.conn)
		//verify client success,send self's verify message to client
		verifymsg := makeVerifyMsg(p.servername, verifydata, p.starttime)
		send := 0
		for send < len(verifymsg) {
			num, e := conn.Write(verifymsg[send:])
			if e != nil {
				fmt.Printf("[Stream.TCP.sworker] write first verify message error:%s to ip:%s\n", e, conn.RemoteAddr().String())
				p.parentnode.Lock()
				delete(p.parentnode.peers, p.clientname+p.servername)
				p.closeconn()
				p.status = false
				p.parentnode.Unlock()
				this.putpeer(p)
				return false
			}
			send += num
		}
		//p.writerbuffer <- verifymsg
		if this.onlinefunc != nil {
			switch p.selftype {
			case CLIENT:
				this.onlinefunc(p, p.servername, p.starttime)
			case SERVER:
				this.onlinefunc(p, p.clientname, p.starttime)
			}
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		this.putpeer(p)
		return false
	}
}

func (this *Instance) StartTcpClient(selfname string, verifydata []byte, serveraddr string) bool {
	c, e := net.DialTimeout("tcp", serveraddr, time.Second)
	if e != nil {
		fmt.Printf("[Stream.StartTcpClient]tcp connect server addr:%s error:%s\n", serveraddr, e)
		return false
	}
	p := this.getpeer()
	p.protocoltype = TCP
	p.clientname = selfname
	p.selftype = CLIENT
	p.conn = unsafe.Pointer(c.(*net.TCPConn))
	p.setbuffer(this.conf.MinReadBufferLen)
	p.status = true
	p.starttime = time.Now().UnixNano()
	return this.cworker(p, verifydata)
}
func (this *Instance) cworker(p *Peer, verifydata []byte) bool {
	//send self's verify message to server
	conn := (*net.TCPConn)(p.conn)
	verifymsg := makeVerifyMsg(p.clientname, verifydata, p.starttime)
	send := 0
	for send < len(verifymsg) {
		num, e := conn.Write(verifymsg[send:])
		if e != nil {
			fmt.Printf("[Stream.TCP.cworker] write first verify message error:%s to ip:%s\n", e, conn.RemoteAddr().String())
			return false
		}
		send += num
	}
	//read first verify message from server
	this.verifypeer(p, verifydata)
	if p.servername != "" {
		//verify server success
		if this.onlinefunc != nil {
			switch p.selftype {
			case CLIENT:
				this.onlinefunc(p, p.servername, p.starttime)
			case SERVER:
				this.onlinefunc(p, p.clientname, p.starttime)
			}
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		this.putpeer(p)
		return false
	}
}
func (this *Instance) verifypeer(p *Peer, verifydata []byte) {
	conn := (*net.TCPConn)(p.conn)
	conn.SetReadDeadline(time.Now().Add(time.Duration(this.conf.VerifyTimeout) * time.Millisecond))
	var e error
	for {
		if p.readbuffer.Rest() >= len(p.tempbuffer) {
			p.tempbuffernum, e = conn.Read(p.tempbuffer)
		} else {
			p.tempbuffernum, e = conn.Read(p.tempbuffer[:p.readbuffer.Rest()])
		}
		if e != nil {
			switch p.selftype {
			case CLIENT:
				fmt.Printf("[Stream.TCP.cworker] read first verify message error:%s from ip:%s\n", e, conn.RemoteAddr().String())
			case SERVER:
				fmt.Printf("[Stream.TCP.sworker] read first verify message error:%s from ip:%s\n", e, conn.RemoteAddr().String())
			}
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
					switch p.selftype {
					case CLIENT:
						fmt.Printf("[Stream.TCP.cworker] message too large form ip:%s\n", conn.RemoteAddr().String())
					case SERVER:
						fmt.Printf("[Stream.TCP.sworker] message too large form ip:%s\n", conn.RemoteAddr().String())
					}
					return
				}
				break
			}
			msg := &TotalMsg{}
			if e := proto.Unmarshal(p.readbuffer.Get(int(msglen + 2))[2:], msg); e != nil {
				switch p.selftype {
				case CLIENT:
					fmt.Printf("[Stream.TCP.cworker] message wrong form ip:%s\n", conn.RemoteAddr().String())
				case SERVER:
					fmt.Printf("[Stream.TCP.sworker] message wrong form ip:%s\n", conn.RemoteAddr().String())
				}
				return
			}
			//first message must be verify message
			if msg.Totaltype != TotalMsgType_VERIFY {
				continue
			}
			var node *peernode
			var ok bool
			switch p.selftype {
			case CLIENT:
				//client drop data race message
				if p.starttime != msg.Starttime {
					continue
				}
				if msg.Sender == "" || msg.Sender == p.clientname {
					fmt.Printf("[Stream.TCP.cworker] name check error from ip:%s\n", conn.RemoteAddr().String())
					return
				}
				if !this.verifyfunc(p.clientname, verifydata, msg.Sender, msg.GetVerify().Verifydata) {
					fmt.Printf("[Stream.TCP.cworker] verify failed with data:%s from ip:%s\n", msg.GetVerify().Verifydata, conn.RemoteAddr().String())
					return
				}
				node = this.peernodes[this.getindex(p.clientname+msg.Sender)]
				node.Lock()
				_, ok = node.peers[p.clientname+msg.Sender]
			case SERVER:
				if msg.Sender == "" || msg.Sender == p.servername {
					fmt.Printf("[Stream.TCP.sworker] name check error from ip:%s\n", conn.RemoteAddr().String())
				}
				if !this.verifyfunc(p.servername, verifydata, msg.Sender, msg.GetVerify().Verifydata) {
					fmt.Printf("[Stream.TCP.sworker] verify failed with data:%s from ip:%s\n", msg.GetVerify().Verifydata, conn.RemoteAddr().String())
					return
				}
				node = this.peernodes[this.getindex(msg.Sender+p.servername)]
				node.Lock()
				_, ok = node.peers[msg.Sender+p.servername]
			}
			if ok {
				node.Unlock()
				return
			}
			p.parentnode = node
			p.starttime = msg.Starttime
			p.lastactive = time.Now().UnixNano()
			switch p.selftype {
			case CLIENT:
				p.servername = msg.Sender
			case SERVER:
				p.clientname = msg.Sender
			}
			node.peers[p.clientname+p.servername] = p
			node.Unlock()
			return
		}
	}
}
func (this *Instance) read(p *Peer) {
	conn := (*net.TCPConn)(p.conn)
	defer func() {
		if p.parentnode == nil {
			fmt.Println("nil")
		}
		p.parentnode.Lock()
		//every connection will have two goruntine to work for it
		if _, ok := p.parentnode.peers[p.clientname+p.servername]; ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.clientname+p.servername)
			//cause write goruntine return,this will be useful when there is nothing in writebuffer
			p.writerbuffer <- []byte{}
			p.status = false
			p.parentnode.Unlock()
		} else {
			p.parentnode.Unlock()
			//when second goruntine return,put connection back to the pool
			if this.offlinefunc != nil {
				switch p.selftype {
				case CLIENT:
					this.offlinefunc(p, p.servername, p.starttime)
				case SERVER:
					this.offlinefunc(p, p.clientname, p.starttime)
				}
			}
			this.putpeer(p)
		}
	}()
	//after verify,the read timeout is useless,heartbeat will work for this
	conn.SetReadDeadline(time.Time{})
	var e error
	for {
		if p.readbuffer.Rest() >= len(p.tempbuffer) {
			p.tempbuffernum, e = conn.Read(p.tempbuffer)
		} else {
			p.tempbuffernum, e = conn.Read(p.tempbuffer[:p.readbuffer.Rest()])
		}
		if e != nil {
			if e != io.EOF {
				switch p.selftype {
				case CLIENT:
					fmt.Printf("[Stream.TCP.read] read data error:%s from server:%s ip:%s\n", e, p.servername, conn.RemoteAddr().String())
				case SERVER:
					fmt.Printf("[Stream.TCP.read] read data error:%s from client:%s ip:%s\n", e, p.clientname, conn.RemoteAddr().String())
				}
			}
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
					switch p.selftype {
					case CLIENT:
						fmt.Printf("[Stream.TCP.read] message too large form server:%s ip:%s\n", p.servername, conn.RemoteAddr().String())
					case SERVER:
						fmt.Printf("[Stream.TCP.read] message too large form client:%s ip:%s\n", p.clientname, conn.RemoteAddr().String())
					}
					return
				}
				break
			}
			msg := &TotalMsg{}
			if e = proto.Unmarshal(p.readbuffer.Get(int(msglen + 2))[2:], msg); e != nil {
				switch p.selftype {
				case CLIENT:
					fmt.Printf("[Stream.TCP.read] message wrong form server:%s ip:%s\n", p.servername, conn.RemoteAddr().String())
				case SERVER:
					fmt.Printf("[Stream.TCP.read] message wrong form client:%s ip:%s\n", p.clientname, conn.RemoteAddr().String())
				}
				return
			}
			//drop data race message
			if msg.Starttime != p.starttime {
				continue
			}
			switch p.selftype {
			case CLIENT:
				if msg.Sender != p.servername {
					continue
				}
			case SERVER:
				if msg.Sender != p.clientname {
					continue
				}
			}
			//deal message
			switch msg.Totaltype {
			case TotalMsgType_HEART:
				p.lastactive = time.Now().UnixNano()
			case TotalMsgType_USER:
				p.lastactive = time.Now().UnixNano()
				switch p.selftype {
				case CLIENT:
					this.userdatafunc(p, p.servername, p.starttime, msg.GetUser().Userdata)
				case SERVER:
					this.userdatafunc(p, p.clientname, p.starttime, msg.GetUser().Userdata)
				}
			}
		}
	}
}
func (this *Instance) write(p *Peer) {
	conn := (*net.TCPConn)(p.conn)
	defer func() {
		if p.parentnode == nil {
			fmt.Println("nil")
		}
		p.parentnode.Lock()
		//every connection will have two goruntine to work for it
		if _, ok := p.parentnode.peers[p.clientname+p.servername]; ok {
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
				switch p.selftype {
				case CLIENT:
					this.offlinefunc(p, p.servername, p.starttime)
				case SERVER:
					this.offlinefunc(p, p.clientname, p.starttime)
				}
			}
			this.putpeer(p)
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
			num, e = conn.Write(data[send:])
			if e != nil {
				if e != io.EOF {
					switch p.selftype {
					case CLIENT:
						fmt.Printf("[Stream.TCP.write] write data error:%s to server:%s ip:%s\n", e, p.servername, conn.RemoteAddr().String())
					case SERVER:
						fmt.Printf("[Stream.TCP.write] write data error:%s to client:%s ip:%s\n", e, p.clientname, conn.RemoteAddr().String())
					}
				}
				return
			}
			send += num
		}
	}
}
