package logger

import (
	"Corelib/memqueue"
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
	Timeout int64 //unit millisecond,default 1000,min 200
	Verify  bool
}
type Collector struct {
	c           *CollectorConfig
	wg          *sync.WaitGroup
	cb          LogCallBack
	clker       *sync.Mutex
	closestatus bool

	r *remote
}

//param 1 remoteid
//param 2 remote's addr
//param 2 log content,if disconnect with this server,this is nil
//param 3 logfile name on server,if disconnect with this server,this is empty
//param 4 line num(start from 0)
type LogCallBack func(int64, string, []byte, string, uint32)

func NewCollector(c *CollectorConfig, cb LogCallBack, mq *memqueue.MQ) *Collector {
	if c.Proto != "tcp" && c.Proto != "unix" {
		panic("net proto unknown!\n")
	}
	if c.NetAddr == "" {
		panic("net addr is empty\n")
	}
	if c.Timeout < 200 {
		c.Timeout = DefaultTimeout
	}
	if mq == nil {
		panic("need more memqueue\n")
	}
	if cb == nil {
		panic("log call back is nil\n")
	}
	instance := &Collector{
		c:     c,
		wg:    new(sync.WaitGroup),
		cb:    cb,
		clker: new(sync.Mutex),
	}
	//connect to server
	if strings.ToLower(c.Proto) == "tcp" {
		rtaddr, e := net.ResolveTCPAddr("tcp", c.NetAddr)
		if e != nil {
			panic(fmt.Sprintf("tcpaddr format error:%s\n", e))
		}
		c, e := net.DialTCP(rtaddr.Network(), nil, rtaddr)
		if e != nil {
			panic(fmt.Sprintf("connect tcp server error:%s\n", e))
		}
		//use nagle
		if e = c.SetNoDelay(false); e != nil {
			panic(fmt.Sprintf("set nagle error:%s\n", e))
		}
		r := new(remote)
		r.tcpConn = c
		r.notice = make(chan int, 1)
		r.lastHeart = time.Now().UnixNano()
		r.mq = mq
		instance.r = r
		instance.wg.Add(3)
		instance.workTcp()
	} else if strings.ToLower(c.Proto) == "unix" {
		ruaddr, e := net.ResolveUnixAddr("unix", c.NetAddr)
		if e != nil {
			panic(fmt.Sprintf("unixaddr format error:%s\n", e))
		}
		c, e := net.DialUnix(ruaddr.Network(), nil, ruaddr)
		if e != nil {
			panic(fmt.Sprintf("connect unix server error:%s\n", e))
		}
		r := new(remote)
		r.unixConn = c
		r.notice = make(chan int, 1)
		r.lastHeart = time.Now().UnixNano()
		r.mq = mq
		instance.r = r
		instance.wg.Add(3)
		instance.workUnix()
	}
	return instance
}

func (c *Collector) workTcp() {
	var ew error
	var nw int
	var send int
	writefunc := func(data []byte) bool {
		send = 0
		for {
			if nw, ew = c.r.tcpConn.Write(data[send:]); ew != nil {
				c.r.tcpConn.Close()
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
		if nr, er = c.r.tcpConn.Read(tempbuf); er != nil {
			c.r.tcpConn.Close()
			close(c.r.notice)
			return false
		}
		buffer = append(buffer, tempbuf[:nr]...)
		return true
	}
	if c.c.Verify {
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
					atomic.StoreInt64(&c.r.lastHeart, time.Now().UnixNano())
				case LOG:
					lmsg := msg.GetLog()
					c.r.remoteid = lmsg.Serverid
					go c.cb(c.r.remoteid, c.c.NetAddr, lmsg.Content, lmsg.Filename, lmsg.Line)
					data := makeConfirmMsg(lmsg.Filename, lmsg.Line, lmsg.Memindex)
					c.r.mq.Put(unsafe.Pointer(&data))
				default:
					continue
				}
			}
		}
		c.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return tcp write")
		//write
		for {
			udata, left := c.r.mq.Get(c.r.notice)
			if udata == nil {
				//this is a notice message
				if left == 0 {
					break
				}
			} else {
				//this is a normal message
				if !writefunc(*(*[]byte)(udata)) {
					break
				}
			}
		}
		c.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return tcp heart")
		//heart
		tker := time.NewTicker(time.Duration(c.c.Timeout/3) * time.Millisecond)
		for {
			select {
			case <-c.r.notice:
				c.wg.Done()
				tker.Stop()
				go c.cb(c.r.remoteid, c.c.NetAddr, nil, "", 0)
				return
			case <-tker.C:
				if time.Now().UnixNano()-atomic.LoadInt64(&c.r.lastHeart) > c.c.Timeout*1000*1000 {
					c.r.tcpConn.Close()
					c.wg.Done()
					tker.Stop()
					go c.cb(c.r.remoteid, c.c.NetAddr, nil, "", 0)
					return
				} else {
					data := makeHeartMsg()
					c.r.mq.Put(unsafe.Pointer(&data))
				}
			}
		}
	}()
}
func (c *Collector) workUnix() {
	var ew error
	var nw int
	var send int
	writefunc := func(data []byte) bool {
		send = 0
		for {
			if nw, ew = c.r.unixConn.Write(data[send:]); ew != nil {
				c.r.unixConn.Close()
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
		if nr, er = c.r.unixConn.Read(tempbuf); er != nil {
			c.r.unixConn.Close()
			close(c.r.notice)
			return false
		}
		buffer = append(buffer, tempbuf[:nr]...)
		return true
	}
	if c.c.Verify {
		//TODO
		//verify first
	}
	go func() {
		//defer fmt.Println("return unix read")
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
					atomic.StoreInt64(&c.r.lastHeart, time.Now().UnixNano())
				case LOG:
					lmsg := msg.GetLog()
					c.r.remoteid = lmsg.Serverid
					go c.cb(c.r.remoteid, c.c.NetAddr, lmsg.Content, lmsg.Filename, lmsg.Line)
					data := makeConfirmMsg(lmsg.Filename, lmsg.Line, lmsg.Memindex)
					c.r.mq.Put(unsafe.Pointer(&data))
				default:
					continue
				}
			}
		}
		c.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return unix write")
		//write
		for {
			udata, left := c.r.mq.Get(c.r.notice)
			if udata == nil {
				//this is a notice message
				if left == 0 {
					break
				}
			} else {
				//this is a normal message
				if !writefunc(*(*[]byte)(udata)) {
					break
				}
			}
		}
		c.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return heart")
		//heart
		tker := time.NewTicker(time.Duration(c.c.Timeout/3) * time.Millisecond)
		for {
			select {
			case <-c.r.notice:
				tker.Stop()
				go c.cb(c.r.remoteid, c.c.NetAddr, nil, "", 0)
				c.wg.Done()
				return
			case <-tker.C:
				if time.Now().UnixNano()-atomic.LoadInt64(&c.r.lastHeart) > c.c.Timeout*1000*1000 {
					c.r.unixConn.Close()
					tker.Stop()
					go c.cb(c.r.remoteid, c.c.NetAddr, nil, "", 0)
					c.wg.Done()
					return
				} else {
					data := makeHeartMsg()
					c.r.mq.Put(unsafe.Pointer(&data))
				}
			}
		}
	}()
}
func (c *Collector) Close() {
	c.clker.Lock()
	c.closestatus = true
	c.clker.Unlock()
	if c.r.tcpConn != nil {
		//fmt.Println("close tcp")
		c.r.tcpConn.Close()
	} else {
		//fmt.Println("close unix")
		c.r.unixConn.Close()
	}
	c.wg.Wait()
}
func (c *Collector) Remove(year, month, day, hour int) {
	c.clker.Lock()
	if c.closestatus {
		c.clker.Unlock()
		return
	}
	c.wg.Add(1)
	c.clker.Unlock()
	data := makeRemoveMsg(int32(year), int32(month), int32(day), int32(hour))
	c.r.mq.Put(unsafe.Pointer(&data))
	c.wg.Done()
}
