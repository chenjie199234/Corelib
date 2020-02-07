package logger

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type CollectorConfig struct {
	PeerId  int64  //self id
	Proto   string //tcp or unix
	NetAddr string
}
type Collector struct {
	verify  bool
	timeout int64 //unit millisecond,default 1000,min 200
	wg      *sync.WaitGroup
	cb      LogCallBack

	clker   *sync.Mutex
	closech chan struct{}

	rlker   *sync.Mutex
	remotes map[string]*remote
}

//param 1 remoteid
//param 2 remote's addr
//param 3 log content,if disconnect with this server,this is nil
//param 4 logfile name on server,if disconnect with this server,this is empty
//param 5 offset num in this file(start from 0)
//param 6 start sync time(unixnano)
type LogCallBack func(int64, string, []byte, string, uint32, int64) error

//'timeout' unit millisecond,default 1000,min 200
func NewCollector(timeout int64, verify bool, cb LogCallBack) *Collector {
	if cb == nil {
		panic("log call back is nil\n")
	}
	instance := &Collector{
		verify:  verify,
		wg:      new(sync.WaitGroup),
		cb:      cb,
		clker:   new(sync.Mutex),
		closech: make(chan struct{}, 1),
		rlker:   new(sync.Mutex),
		remotes: make(map[string]*remote),
	}
	if timeout < 200 {
		instance.timeout = DefaultTimeout
	} else {
		instance.timeout = timeout
	}
	//heartbeat check
	go func() {
		tker := time.NewTicker(time.Duration(instance.timeout/3) * time.Millisecond)
		tkercount := 0
		for {
			<-tker.C
			tkercount++
			now := time.Now().UnixNano()
			instance.rlker.Lock()
			for _, r := range instance.remotes {
				//send heartbeat
				select {
				case r.notice <- 1:
				default:
				}
				//check timeout
				if tkercount >= 3 {
					if now-atomic.LoadInt64(&r.lastHeart) > instance.timeout*1000*1000 {
						switch r.netkind {
						case "tcp":
							r.tcpConn.Close()
							go instance.cb(r.remoteid, r.tcpConn.RemoteAddr().String(), nil, "", 0, 0)
						case "unix":
							r.unixConn.Close()
							go instance.cb(r.remoteid, r.unixConn.RemoteAddr().String(), nil, "", 0, 0)
						}
					}
				}
			}
			if tkercount >= 3 {
				tkercount = 0
			}
			instance.rlker.Unlock()
		}
	}()
	return instance
}
func (this *Collector) NewPeer(c *CollectorConfig) error {
	if c == nil {
		return fmt.Errorf("config is empty")
	}
	if c.Proto != "tcp" && c.Proto != "unix" {
		return fmt.Errorf("unknown net kind")
	}
	if c.NetAddr == "" {
		return fmt.Errorf("net addr is empty")
	}
	//connect to server
	this.clker.Lock()
	select {
	case <-this.closech:
		this.clker.Unlock()
		return fmt.Errorf("collector is closing")
	default:
		if strings.ToLower(c.Proto) == "tcp" {
			rtaddr, e := net.ResolveTCPAddr("tcp", c.NetAddr)
			if e != nil {
				return fmt.Errorf("tcpaddr format error:%s", e)
			}
			conn, e := net.DialTCP(rtaddr.Network(), nil, rtaddr)
			if e != nil {
				return fmt.Errorf("connect tcp server error:%s", e)
			}
			//use nagle
			if e = conn.SetNoDelay(false); e != nil {
				return fmt.Errorf("set tcp nagle error:%s", e)
			}
			r := new(remote)
			r.netkind = "tcp"
			r.tcpConn = conn
			r.lastHeart = time.Now().UnixNano()
			r.notice = make(chan uint, 1)
			r.mq = newmq(256)
			this.rlker.Lock()
			this.remotes[conn.RemoteAddr().String()] = r
			this.rlker.Unlock()
			this.wg.Add(2)
			this.workNet(r)
		} else if strings.ToLower(c.Proto) == "unix" {
			ruaddr, e := net.ResolveUnixAddr("unix", c.NetAddr)
			if e != nil {
				return fmt.Errorf("unixaddr format error:%s", e)
			}
			conn, e := net.DialUnix(ruaddr.Network(), nil, ruaddr)
			if e != nil {
				return fmt.Errorf("connect unixsock server error:%s", e)
			}
			r := new(remote)
			r.netkind = "unix"
			r.unixConn = conn
			r.lastHeart = time.Now().UnixNano()
			r.notice = make(chan uint, 1)
			r.mq = newmq(256)
			this.rlker.Lock()
			this.remotes[conn.RemoteAddr().String()] = r
			this.rlker.Unlock()
			this.wg.Add(2)
			this.workNet(r)
		}
	}
	this.clker.Unlock()
	return nil
}
func (this *Collector) workNet(r *remote) {
	var ew error
	var nw int
	var send int
	writefunc := func(data []byte) bool {
		send = 0
		for {
			switch r.netkind {
			case "tcp":
				if nw, ew = r.tcpConn.Write(data[send:]); ew != nil {
					r.tcpConn.Close()
					return false
				}
			case "unix":
				if nw, ew = r.unixConn.Write(data[send:]); ew != nil {
					r.unixConn.Close()
					return false
				}
			default:
				return false
			}
			if nw != len(data) {
				send = nw
			} else {
				break
			}
		}
		return true
	}
	var er error
	var nr int
	buffer := make([]byte, 0)
	tempbuf := make([]byte, 4096)
	readfunc := func() bool {
		switch r.netkind {
		case "tcp":
			if nr, er = r.tcpConn.Read(tempbuf); er != nil {
				r.tcpConn.Close()
				return false
			}
		case "unix":
			if nr, er = r.unixConn.Read(tempbuf); er != nil {
				r.unixConn.Close()
				return false
			}
		default:
			return false
		}
		buffer = append(buffer, tempbuf[:nr]...)
		return true
	}
	if this.verify {
		//TODO
		//verify first
	}
	go func() {
		//defer fmt.Println("return tcp read")
		//read
		for {
			if !readfunc() {
				break
			}
			//deal
			for {
				msg, _ := readFirstMsg(&buffer)
				if msg == nil {
					break
				}
				switch msg.Type {
				case HEART:
					atomic.StoreInt64(&r.lastHeart, time.Now().UnixNano())
				case LOG:
					lmsg := msg.GetLog()
					switch r.netkind {
					case "tcp":
						go func() {
							if this.cb(lmsg.Serverid, r.tcpConn.RemoteAddr().String(), lmsg.Content, lmsg.Filename, lmsg.Offset, lmsg.Synctime) == nil {
								data := makeConfirmMsg(lmsg.Filename, lmsg.Offset, lmsg.Memindex, lmsg.Synctime)
								r.mq.put(unsafe.Pointer(&data))
							}
						}()
					case "unix":
						go func() {
							if this.cb(lmsg.Serverid, r.unixConn.RemoteAddr().String(), lmsg.Content, lmsg.Filename, lmsg.Offset, lmsg.Synctime) == nil {
								data := makeConfirmMsg(lmsg.Filename, lmsg.Offset, lmsg.Memindex, lmsg.Synctime)
								r.mq.put(unsafe.Pointer(&data))
							}
						}()
					default:
						continue
					}
				default:
					continue
				}
			}
		}
		this.rlker.Lock()
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			delete(this.remotes, r.tcpConn.RemoteAddr().String())
		case "unix":
			r.unixConn.Close()
			delete(this.remotes, r.unixConn.RemoteAddr().String())
		}
		this.rlker.Unlock()
		this.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return tcp write")
		//write
		for {
			udata, notice := r.mq.get(r.notice)
			if udata == nil {
				//this is a notice message
				if notice == -1 {
					//closed
					break
				}
				switch notice {
				case 1:
					data := makeHeartMsg()
					if !writefunc(data) {
						break
					}
				}
			} else {
				//this is a normal message
				if !writefunc(*(*[]byte)(udata)) {
					break
				}
			}
		}
		this.rlker.Lock()
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			delete(this.remotes, r.tcpConn.RemoteAddr().String())
		case "unix":
			r.unixConn.Close()
			delete(this.remotes, r.unixConn.RemoteAddr().String())
		}
		this.rlker.Unlock()
		this.wg.Done()
	}()
}
func (this *Collector) ClosePeer(remoteaddr string) {
	this.rlker.Lock()
	if r, ok := this.remotes[remoteaddr]; ok {
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			go this.cb(r.remoteid, r.tcpConn.RemoteAddr().String(), nil, "", 0, 0)
		case "unix":
			r.unixConn.Close()
			go this.cb(r.remoteid, r.unixConn.RemoteAddr().String(), nil, "", 0, 0)
		}
	}
	delete(this.remotes, remoteaddr)
	this.rlker.Unlock()
}

func (this *Collector) CloseCollector() {
	this.clker.Lock()
	close(this.closech)
	this.clker.Unlock()
	this.rlker.Lock()
	for _, r := range this.remotes {
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			go this.cb(r.remoteid, r.tcpConn.RemoteAddr().String(), nil, "", 0, 0)
		case "unix":
			r.unixConn.Close()
			go this.cb(r.remoteid, r.unixConn.RemoteAddr().String(), nil, "", 0, 0)
		}
	}
	this.remotes = make(map[string]*remote)
	this.rlker.Unlock()
	this.wg.Wait()
}

func (this *Collector) Remove(remoteaddr string, year, month, day, hour int) {
	this.clker.Lock()
	select {
	case <-this.closech:
		this.clker.Unlock()
		return
	default:
		this.wg.Add(1)
		defer this.wg.Done()
	}
	this.clker.Unlock()
	this.rlker.Lock()
	if r, ok := this.remotes[remoteaddr]; ok {
		data := makeRemoveMsg(int32(year), int32(month), int32(day), int32(hour))
		r.mq.put(unsafe.Pointer(&data))
	}
	this.rlker.Unlock()
}
