package stream

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"google.golang.org/protobuf/proto"
)

func (this *Instance) StartTcpServer(listenaddr string) {
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
			p.peertype = PEERTCP
			if conn, e = l.AcceptTCP(); e != nil {
				fmt.Printf("[Stream.StartTcpServer]accept tcp connect error:%s\n", e)
				return
			}
			p.conn = unsafe.Pointer(conn)
			p.setbuffer(this.conf.MinReadBufferLen)
			p.status = true
			go this.sworker(p)
		}
	}()
}
func (this *Instance) sworker(p *Peer) bool {
	//read first verify message from client
	this.verifypeer(p, true)
	if p.name != "" {
		//verify client success,send self's verify message to client
		verifymsg := makeVerifyMsg(this.conf.SelfName, this.conf.VerifyData, p.starttime)
		p.writerbuffer <- verifymsg
		if this.onlinefunc != nil {
			this.onlinefunc(p, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		this.putpeer(p)
		return false
	}
}

func (this *Instance) StartTcpClient(addr string) bool {
	c, e := net.DialTimeout("tcp", addr, time.Second)
	if e != nil {
		fmt.Printf("[Stream.StartTcpClient]tcp connect server addr:%s error:%s\n", addr, e)
		return false
	}
	p := this.getpeer()
	p.peertype = PEERTCP
	p.conn = unsafe.Pointer(c.(*net.TCPConn))
	p.setbuffer(this.conf.MinReadBufferLen)
	p.starttime = time.Now().UnixNano()
	p.status = true
	return this.cworker(p)
}
func (this *Instance) cworker(p *Peer) bool {
	//send self's verify message to server
	conn := (*net.TCPConn)(p.conn)
	verifymsg := makeVerifyMsg(this.conf.SelfName, this.conf.VerifyData, p.starttime)
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
	this.verifypeer(p, false)
	if p.name != "" {
		//verify server success
		if this.onlinefunc != nil {
			this.onlinefunc(p, p.starttime)
		}
		go this.read(p)
		go this.write(p)
		return true
	} else {
		this.putpeer(p)
		return false
	}
}
func (this *Instance) verifypeer(p *Peer, sorc bool) {
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
			if sorc {
				fmt.Printf("[Stream.TCP.sworker] read first verify message error:%s from ip:%s\n", e, conn.RemoteAddr().String())
			} else {
				fmt.Printf("[Stream.TCP.cworker] read first verify message error:%s from ip:%s\n", e, conn.RemoteAddr().String())
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
					if sorc {
						fmt.Printf("[Stream.TCP.sworker] message too large form ip:%s\n", conn.RemoteAddr().String())
					} else {
						fmt.Printf("[Stream.TCP.cworker] message too large form ip:%s\n", conn.RemoteAddr().String())
					}
					return
				}
				break
			}
			msg := &TotalMsg{}
			if e := proto.Unmarshal(p.readbuffer.Get(int(msglen + 2))[2:], msg); e != nil {
				if sorc {
					fmt.Printf("[Stream.TCP.sworker] message wrong form ip:%s\n", conn.RemoteAddr().String())
				} else {
					fmt.Printf("[Stream.TCP.cworker] message wrong form ip:%s\n", conn.RemoteAddr().String())
				}
				return
			}
			if msg.Totaltype != TotalMsgType_VERIFY {
				continue
			}
			if msg.Sender == "" {
				if sorc {
					fmt.Printf("[Stream.TCP.sworker] empty sender name from ip:%s\n", conn.RemoteAddr().String())
				} else {
					fmt.Printf("[Stream.TCP.cworker] empty sender name from ip:%s\n", conn.RemoteAddr().String())
				}
				return
			}
			if !sorc && p.starttime != msg.Starttime {
				fmt.Printf("[Stream.TCP.cworker] connection starttime error from ip:%s\n", conn.RemoteAddr().String())
				return
			}
			if !this.verifyfunc(this.conf.VerifyData, msg.GetVerify().Verifydata, msg.Sender) {
				if sorc {
					fmt.Printf("[Stream.TCP.sworker] verify failed with data:%s from ip:%s\n", msg.GetVerify().Verifydata, conn.RemoteAddr().String())
				} else {
					fmt.Printf("[Stream.TCP.cworker] verify failed with data:%s from ip:%s\n", msg.GetVerify().Verifydata, conn.RemoteAddr().String())
				}
				return
			}
			node := this.peernodes[this.getindex(msg.Sender)]
			node.Lock()
			if _, ok := node.peers[msg.Sender]; ok {
				node.Unlock()
				return
			}
			p.parentnode = node
			p.name = msg.Sender
			p.starttime = msg.Starttime
			p.lastactive = time.Now().UnixNano()
			node.peers[p.name] = p
			node.Unlock()
			return
		}
	}
}
func (this *Instance) read(p *Peer) {
	conn := (*net.TCPConn)(p.conn)
	defer func() {
		//every connection will have two goruntine to work for it
		p.parentnode.Lock()
		if _, ok := p.parentnode.peers[p.name]; ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.name)
			//cause write goruntine return,this will be useful when there is nothing in writebuffer
			p.writerbuffer <- []byte{}
			p.status = false
		} else {
			//when second goruntine return,put connection back to the pool
			if this.offlinefunc != nil {
				this.offlinefunc(p, p.starttime)
			}
			this.putpeer(p)
		}
		p.parentnode.Unlock()
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
			fmt.Printf("[Stream.TCP.read] read data error:%s from ip:%s\n", e, conn.RemoteAddr().String())
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
					fmt.Printf("[Stream.TCP.read] message too large form ip:%s\n", conn.RemoteAddr().String())
					return
				}
				break
			}
			msg := &TotalMsg{}
			if e = proto.Unmarshal(p.readbuffer.Get(int(msglen + 2))[2:], msg); e != nil {
				fmt.Printf("[Stream.TCP.read] message wrong form ip:%s\n", conn.RemoteAddr().String())
				return
			}
			if msg.Sender != p.name || msg.Starttime != p.starttime {
				continue
			}
			switch msg.Totaltype {
			case TotalMsgType_HEART:
				p.lastactive = time.Now().UnixNano()
			case TotalMsgType_USER:
				p.lastactive = time.Now().UnixNano()
				this.userdatafunc(p, p.starttime, msg.GetUser().Userdata)
			}
		}
	}
}
func (this *Instance) write(p *Peer) {
	conn := (*net.TCPConn)(p.conn)
	defer func() {
		//every connection will have two goruntine to work for it
		p.parentnode.Lock()
		if _, ok := p.parentnode.peers[p.name]; ok {
			//when first goruntine return,delete this connection from the map
			delete(p.parentnode.peers, p.name)
			//close the connection,cause read goruntine return
			p.closeconn()
			p.status = false
		} else {
			//when second goruntine return,put connection back to the pool
			if this.offlinefunc != nil {
				this.offlinefunc(p, p.starttime)
			}
			this.putpeer(p)
		}
		p.parentnode.Unlock()
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
				fmt.Printf("[Stream.TCP.write] write data error:%s to ip:%s\n", e, conn.RemoteAddr().String())
				return
			}
			send += num
		}
	}
}
