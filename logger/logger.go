package logger

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type LoggerConfig struct {
	PeerId int64
	//net
	TcpAddr  string
	UnixAddr string
	BufNum   uint32 //keep how many messages(unit line) in buffer for net writing,default 512,min 128
	Timeout  int64  //unit millisecond,default 1000,min 200
	Verify   bool
	//local
	LocalDir  string
	SplitSize int64 //unit M,default 100M,min 20M
	//std
	Std bool
}
type Logger struct {
	c       *LoggerConfig   //config
	wg      *sync.WaitGroup //wait all worker to close
	clker   *sync.Mutex
	closech chan struct{}

	locallogfile *os.File //local log file
	fmq          *MQ      //mem message queue for local log
	flker        *sync.Mutex
	finfos       []os.FileInfo
	fileindex    int

	netinfofile  *os.File
	netlogfile   *os.File
	netlogreader *bufio.Reader
	nmq          *MQ

	update       chan struct{}
	refresh      chan struct{}
	msglist      []*msg
	msgheadindex uint32
	msgtailindex uint32
	msgcount     uint32

	tcpListener  *net.TCPListener
	unixListener *net.UnixListener
	rlker        *sync.Mutex
	remotes      map[string]*remote
}

type msg struct {
	content  []byte
	filename string //file name
	line     uint32
	offset   uint32 //end offset of this msg
	done     bool
	memindex uint32
}

//first memmq is for local log
//second memmq is for net log
func NewLogger(c *LoggerConfig, mqs ...*MQ) *Logger {
	context.Background()
	//check config
	if c == nil || (c.TcpAddr == "" && c.UnixAddr == "" && c.LocalDir == "" && !c.Std) {
		panic("config is empty\n")
	}
	if c.TcpAddr != "" || c.UnixAddr != "" {
		if len(mqs) < 2 {
			panic("need more memqueue\n")
		}
	} else if c.LocalDir != "" {
		if len(mqs) < 1 {
			panic("need more memqueue\n")
		}
	}
	//the net log is based on the local log
	if c.TcpAddr != "" || c.UnixAddr != "" {
		if c.LocalDir == "" {
			c.LocalDir = DefaultLocalDir
		}
		if c.Timeout < 200 {
			c.Timeout = DefaultTimeout
		}
		if c.BufNum < 128 {
			c.BufNum = DefaultNetLogNum
		}
	}
	if c.LocalDir != "" && c.SplitSize < 1 {
		c.SplitSize = DefaultSplitSize
	}
	//new logger
	instance := &Logger{
		c:       c,
		wg:      new(sync.WaitGroup),
		clker:   new(sync.Mutex),
		closech: make(chan struct{}, 1),
	}
	if c.LocalDir != "" {
		instance.fmq = mqs[0]
		instance.flker = new(sync.Mutex)
		instance.newLocal()
		if c.TcpAddr != "" || c.UnixAddr != "" {
			instance.nmq = mqs[1]
			instance.refresh = make(chan struct{}, 1)
			instance.update = make(chan struct{}, 1)
			instance.msglist = make([]*msg, c.BufNum)
			instance.msgheadindex = 0
			instance.msgtailindex = 0
			instance.msgcount = 0
			instance.rlker = new(sync.Mutex)
			instance.remotes = make(map[string]*remote)
			instance.newNet()
		}
	}
	return instance
}
func (l *Logger) newLocal() {
	//create log dir
	if finfo, e := os.Stat(l.c.LocalDir); e != nil {
		if os.IsNotExist(e) {
			if e := os.Mkdir(l.c.LocalDir, 0755); e != nil {
				panic(fmt.Sprintf("logdir doesn't exist and create it error:%s\n", e))
			}
		} else {
			panic(fmt.Sprintf("get logdir's info error:%s\n", e))
		}
	} else if !finfo.IsDir() {
		panic("logdir's path link to an exist file which is not a dir\n")
	}
	//get all old logfile's info in log dir
	//finfos are sorted by file name
	finfos, e := ioutil.ReadDir(l.c.LocalDir)
	if e != nil {
		panic(fmt.Sprintf("read all files in logdir error:%s\n", e))
	}
	//remove all unexpected files and dirs in log dir
	curpos := 0
	for i, finfo := range finfos {
		tempfilename := filepath.Base(finfo.Name())
		if _, e = time.Parse(timeformat, tempfilename); e == nil && !finfo.IsDir() && tempfilename[0] != '.' {
			if curpos != i {
				finfos[curpos] = finfo
			}
			curpos++
		}
	}
	l.finfos = finfos[:curpos]

	if len(l.finfos) == 0 || l.finfos[len(l.finfos)-1].Size()/1024/1024 > l.c.SplitSize {
		//create new logfile
		var e error
		logfilename := l.c.LocalDir + "/" + newLogfileName()
		if l.locallogfile, e = os.OpenFile(logfilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); e != nil {
			if l.locallogfile != nil {
				l.locallogfile.Close()
				if ee := os.Remove(logfilename); ee != nil {
					panic(fmt.Sprintf("create new logfile error:%s\n"+
						"delete it error:%s\n"+
						"please delete it manually\n", e, ee))
				}
			}
			panic(fmt.Sprintf("create new logfile error:%s\n", e))
		}
		var tempfinfo os.FileInfo
		if tempfinfo, e = l.locallogfile.Stat(); e != nil {
			panic(fmt.Sprintf("get new logfile's fileinfo error:%s\n", e))
		}
		l.finfos = append(l.finfos, tempfinfo)
	} else {
		//use this old logfile
		var e error
		logfilename := l.c.LocalDir + "/" + filepath.Base(l.finfos[len(l.finfos)-1].Name())
		if l.locallogfile, e = os.OpenFile(logfilename, os.O_WRONLY|os.O_APPEND, 0644); e != nil {
			panic(fmt.Sprintf("open old logfile error:%s\n", e))
		}
	}
	l.wg.Add(1)
	go l.workFile()
}

func (l *Logger) newNet() {
	var curfilename string
	var curfileoffset uint32
	var curline uint32
	tempinfo, e := os.Stat(l.c.LocalDir + "/.netlog")
	if e != nil && !os.IsNotExist(e) {
		panic(fmt.Sprintf("get .netlog fileinfo error:%s\n", e))
	} else if e == nil {
		//there is a .netlog file
		if tempinfo.IsDir() {
			panic(".netlog is a dir\n")
		}
		if l.netinfofile, e = os.OpenFile(l.c.LocalDir+"/.netlog", os.O_RDWR, 0644); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
			}
			panic(fmt.Sprintf("open .netlog file error:%s\n", e))
		}

		//read .netlog
		var d []byte
		if d, e = ioutil.ReadAll(l.netinfofile); e != nil {
			panic(fmt.Sprintf("read .netlog file error:%s\n", e))
		}
		if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
			}
			panic(fmt.Sprintf("seek .netlog file after readall error:%s\n", e))
		}
		curfileoffset, curline, curfilename = parseLoginfo(d)
		if curfilename == "" {
			panic(".netlog file data broken,please fix it manually\n")
		}

		//find the local logfile
		find := false
		tempindex := 0
		for _, finfo := range l.finfos {
			tempfilename := filepath.Base(finfo.Name())
			if tempfilename >= curfilename {
				if tempfilename != curfilename {
					curfilename = tempfilename
					l.fileindex = tempindex
					curfileoffset = 0
					curline = 0
					var n int
					if n, e = l.netinfofile.Write(serializeLoginfo(0, 0, curfilename)); e != nil {
						l.netinfofile.Close()
						if n != 0 {
							panic(fmt.Sprintf("rewrite .netlog file error:%s\n"+
								"data dirty,please fix it manually\n"+
								"current logfile name:%s\n", e, tempfilename))
						}
						panic(fmt.Sprintf("rewrite .netlog file error:%s\n", e))
					}
					if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
						l.netinfofile.Close()
						panic(fmt.Sprintf("seek .netlog file after rewrite error:%s\n", e))
					}
				}
				find = true
				break
			}
			tempindex++
		}
		if !find {
			panic("logfile name in .netlog is newer then all the logfiles,this is impossible\n")
		}
	} else {
		//there is no .netlog file
		if l.netinfofile, e = os.OpenFile(l.c.LocalDir+"/.netlog", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
				if ee := os.Remove(l.c.LocalDir + "/.netlog"); ee != nil {
					panic(fmt.Sprintf("create .netlog file error:%s\n"+
						"delete it error:%s\n"+
						"please delete it manually\n", e, ee))
				}
			}
			panic(fmt.Sprintf("create .netlog file error:%s\n", e))
		}
		curfilename = filepath.Base(l.finfos[0].Name())
		l.fileindex = 0
		curfileoffset = 0
		curline = 0
		if _, e := l.netinfofile.Write(serializeLoginfo(0, 0, curfilename)); e != nil {
			l.netinfofile.Close()
			if ee := os.Remove(l.c.LocalDir + "/.netlog"); ee != nil {
				panic(fmt.Sprintf("init .netlog file error:%s\n"+
					"delete it error:%s\n"+
					"please delete it manually\n", e, ee))
			}
			panic(fmt.Sprintf("init .netlog file error:%s\n", e))
		}
		if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
			l.netinfofile.Close()
			panic(fmt.Sprintf("seek .netlog file after init error:%s\n", e))
		}
	}
	//open logfile for net read
	if l.netlogfile, e = os.Open(l.c.LocalDir + "/" + curfilename); e != nil {
		if l.netlogfile != nil {
			l.netlogfile.Close()
		}
		panic(fmt.Sprintf("open logfile for net read error:%s\n", e))
	}
	if _, e = l.netlogfile.Seek(int64(curfileoffset), os.SEEK_SET); e != nil {
		l.netlogfile.Close()
		panic(fmt.Sprintf("seek logfile before net read error:%s\n", e))
	}
	l.netlogreader = bufio.NewReader(l.netlogfile)
	//check heart
	l.wg.Add(1)
	go func() {
		//defer fmt.Println("return heart check")
		tker := time.NewTicker(time.Duration(l.c.Timeout) * time.Millisecond)
		for {
			select {
			case <-l.closech:
				l.wg.Done()
				return
			case <-tker.C:
				l.rlker.Lock()
				for _, r := range l.remotes {
					if time.Now().UnixNano()-atomic.LoadInt64(&r.lastHeart) > l.c.Timeout*1000*1000 {
						if r.tcpConn != nil {
							r.tcpConn.Close()
						} else {
							r.unixConn.Close()
						}
					}
				}
				l.rlker.Unlock()
			}
		}
	}()
	//deal net log
	l.wg.Add(1)
	l.refresh <- struct{}{}
	go func() {
		//defer fmt.Println("return net read file")
		var e error
		var n int
		var retry int
		buf := make([]byte, 1)
		content := make([]byte, 1024)
		contentOffset := uint32(0)
		for {
			select {
			case <-l.closech:
				l.wg.Done()
				return
			case <-l.update:
				changed := false
				for {
					if l.msglist[l.msgheadindex] != nil && l.msglist[l.msgheadindex].done {
						changed = true
						l.msgcount--
						l.msgheadindex++
						if l.msgheadindex >= l.c.BufNum {
							l.msgheadindex = 0
						}
						if l.msgheadindex == l.msgtailindex {
							break
						}
					} else {
						break
					}
				}
				if changed {
					retry = 0
					var nextoffset uint32
					var nextline uint32
					var nextfilename string
					if l.msgheadindex == l.msgtailindex {
						//finish all net log in buf
						var tempindex uint32
						if l.msgheadindex == 0 {
							tempindex = l.c.BufNum - 1
						} else {
							tempindex = l.msgheadindex - 1
						}
						nextfilename = l.msglist[tempindex].filename
						nextline = l.msglist[tempindex].line + 1
						nextoffset = l.msglist[tempindex].offset + uint32(len(l.msglist[tempindex].content)) + 1 //add 1 for '\n'
					} else {
						nextfilename = l.msglist[l.msgheadindex].filename
						nextline = l.msglist[l.msgheadindex].line
						nextoffset = l.msglist[l.msgheadindex].offset
					}
					if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
						panic(fmt.Sprintf("seek before rewrite .netlog error:%s\n", e))
					}
					if n, e = l.netinfofile.Write(serializeLoginfo(nextoffset, nextline, nextfilename)); e != nil {
						if l.netinfofile != nil {
							l.netinfofile.Close()
						}
						if n != 0 {
							panic(fmt.Sprintf("rewrite .netlog error:%s\n"+
								"data dirty,please fix it manully\n"+
								"current logfile name:%s\n", e, nextfilename))
						}
						panic(fmt.Sprintf("rewrite .netlog error:%s\n", e))
					}
				} else {
					retry++
					if retry >= 5 || retry >= len(l.msglist)/2 {
						retry = 0
						l.nmq.Put(unsafe.Pointer(l.msglist[0]))
					}
				}
			case <-l.refresh:
				for {
					if _, e = l.netlogfile.Read(buf); e != nil {
						if e == io.EOF {
							locfilename := filepath.Base(l.locallogfile.Name())
							if curfilename != locfilename {
								//local log file changed to a new file
								if contentOffset != 0 {
									panic(fmt.Sprintf("unexpected EOF on logfile:%s\n", curfilename))
								} else {
									//this logfile complete send,search next
									l.flker.Lock()
									l.fileindex++
									curfilename = filepath.Base(l.finfos[l.fileindex].Name())
									curfileoffset = 0
									curline = 0
									var tempfile *os.File
									if tempfile, e = os.Open(l.c.LocalDir + "/" + curfilename); e != nil {
										if tempfile != nil {
											tempfile.Close()
										}
										panic(fmt.Sprintf("open logfile for net read error:%s\n", e))
									}
									go l.netlogfile.Close()
									l.netlogfile = tempfile
									l.netlogreader = bufio.NewReader(l.netlogfile)
									l.flker.Unlock()
									if l.msgcount < l.c.BufNum {
										continue
									}
								}
							}
							break
						} else {
							panic(fmt.Sprintf("read logfile for net log error:%s\n", e))
						}
					} else if buf[0] != '\n' {
						if contentOffset < uint32(len(content)) {
							content[contentOffset] = buf[0]
						} else {
							content = append(content, make([]byte, 1024)...)
							content[contentOffset] = buf[0]
						}
						contentOffset++
					} else {
						message := &msg{
							content:  content[:contentOffset],
							filename: curfilename,
							line:     curline,
							offset:   curfileoffset,
							done:     false,
							memindex: l.msgtailindex,
						}
						l.msglist[l.msgtailindex] = message
						l.msgtailindex++
						l.msgcount++
						if l.msgtailindex >= l.c.BufNum {
							l.msgtailindex = 0
						}
						curline++
						curfileoffset += (contentOffset + 1) //add 1 for '\n'
						contentOffset = 0
						l.nmq.Put(unsafe.Pointer(message))
						if l.msgcount >= l.c.BufNum {
							content = content[:1024]
							break
						}
					}
				}
			}
		}
	}()
	if l.c.TcpAddr != "" {
		//deal tcp net connect
		l.wg.Add(1)
		ltaddr, e := net.ResolveTCPAddr("tcp", l.c.TcpAddr)
		if e != nil {
			panic(fmt.Sprintf("tcp addr format error:%s\n", e))
		}
		l.tcpListener, e = net.ListenTCP(ltaddr.Network(), ltaddr)
		if e != nil {
			panic(fmt.Sprintf("listen tcp error:%s\n", e))
		}
		go func() {
			//defer fmt.Println("return tcp server")
			for {
				c, e := l.tcpListener.AcceptTCP()
				if e != nil {
					break
				}
				//use nagle
				if e = c.SetNoDelay(false); e != nil {
					c.Close()
					continue
				}
				r := new(remote)
				r.tcpConn = c
				r.notice = make(chan int, 1)
				r.lastHeart = time.Now().UnixNano()
				l.rlker.Lock()
				l.remotes[c.RemoteAddr().String()] = r
				l.rlker.Unlock()
				l.wg.Add(3)
				go l.workTcp(r)
			}
			l.wg.Done()
		}()
	}
	if l.c.UnixAddr != "" {
		//deal unix net connect
		l.wg.Add(1)
		luaddr, e := net.ResolveUnixAddr("unix", l.c.UnixAddr)
		if e != nil {
			panic(fmt.Sprintf("unix addr format error:%s\n", e))
		}
		l.unixListener, e = net.ListenUnix(luaddr.Network(), luaddr)
		if e != nil {
			panic(fmt.Sprintf("listen unix error:%s\n", e))
		}
		go func() {
			//defer fmt.Println("return unix server")
			for {
				c, e := l.unixListener.AcceptUnix()
				if e != nil {
					break
				}
				r := new(remote)
				r.unixConn = c
				r.notice = make(chan int)
				r.lastHeart = time.Now().UnixNano()
				l.rlker.Lock()
				l.remotes[c.RemoteAddr().String()] = r
				l.rlker.Unlock()
				l.wg.Add(3)
				go l.workUnix(r)
			}
			l.wg.Done()
		}()
	}
}
func parseLoginfo(d []byte) (uint32, uint32, string) {
	if len(d) != 16 {
		return 0, 0, ""
	}
	return binary.BigEndian.Uint32(d[:4]), binary.BigEndian.Uint32(d[4:8]), time.Unix(int64(binary.BigEndian.Uint64(d[8:16])), 0).UTC().Format(timeformat)
}
func serializeLoginfo(offset uint32, line uint32, filename string) []byte {
	offsetb := make([]byte, 4)
	lineb := make([]byte, 4)
	timeb := make([]byte, 8)
	binary.BigEndian.PutUint32(offsetb, offset)
	binary.BigEndian.PutUint32(lineb, line)
	t, _ := time.Parse(timeformat, filename)
	binary.BigEndian.PutUint64(timeb, uint64(t.Unix()))
	return append(offsetb, append(lineb, timeb...)...)
}

func (l *Logger) workTcp(r *remote) {
	var ew error
	var nw int
	var sendnum int
	writefunc := func(data []byte) bool {
		sendnum = 0
		for {
			if nw, ew = r.tcpConn.Write(data[sendnum:]); ew != nil {
				r.tcpConn.Close()
				return false
			}
			if nw < len(data) {
				sendnum = nw
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
		if nr, er = r.tcpConn.Read(tempbuf); er != nil {
			r.tcpConn.Close()
			close(r.notice)
			return false
		}
		buffer = append(buffer, tempbuf[:nr]...)
		return true
	}
	if l.c.Verify {
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
					select {
					case r.notice <- 1:
					default:
					}
				case CONFIRM:
					cmsg := msg.GetConfirm()
					if l.msglist[cmsg.Memindex].filename == cmsg.Filename && l.msglist[cmsg.Memindex].line == cmsg.Line {
						l.msglist[cmsg.Memindex].done = true
					}
					select {
					case l.update <- struct{}{}:
					default:
					}
				case REMOVE:
					rmsg := msg.GetRemove()
					go l.Remove(int(rmsg.Year), int(rmsg.Month), int(rmsg.Day), int(rmsg.Hour))
				default:
					continue
				}
			}
		}
		l.rlker.Lock()
		delete(l.remotes, r.tcpConn.RemoteAddr().String())
		l.rlker.Unlock()
		l.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return tcp write")
		//write
		for {
			udata, left := l.nmq.Get(r.notice)
			if udata == nil {
				//this is a notice message
				if left == 0 {
					break
				}
				data := makeHeartMsg()
				if !writefunc(data) {
					break
				}
			} else {
				//this is a log message
				if left == 0 {
					select {
					case l.refresh <- struct{}{}:
					default:
					}
				}
				m := (*msg)(udata)
				data := makeLogMsg(l.c.PeerId, m.content, m.filename, m.line, m.memindex)
				if !writefunc(data) {
					//put message back
					l.nmq.Put(udata)
					break
				}
			}
		}
		l.rlker.Lock()
		delete(l.remotes, r.tcpConn.RemoteAddr().String())
		l.rlker.Unlock()
		l.wg.Done()
	}()
}
func (l *Logger) workUnix(r *remote) {
	var ew error
	var nw int
	var sendnum int
	writefunc := func(data []byte) bool {
		sendnum = 0
		for {
			if nw, ew = r.unixConn.Write(data[sendnum:]); ew != nil {
				r.unixConn.Close()
				return false
			}
			if nw < len(data) {
				sendnum = nw
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
		if nr, er = r.unixConn.Read(tempbuf); er != nil {
			r.unixConn.Close()
			close(r.notice)
			return false
		}
		buffer = append(buffer, tempbuf[:nr]...)
		return true
	}
	if l.c.Verify {
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
					atomic.StoreInt64(&r.lastHeart, time.Now().UnixNano())
					select {
					case r.notice <- 1:
					default:
					}
				case CONFIRM:
					cmsg := msg.GetConfirm()
					if l.msglist[cmsg.Memindex].filename == cmsg.Filename && l.msglist[cmsg.Memindex].line == cmsg.Line {
						l.msglist[cmsg.Memindex].done = true
					}
					select {
					case l.update <- struct{}{}:
					default:
					}
				case REMOVE:
					rmsg := msg.GetRemove()
					go l.Remove(int(rmsg.Year), int(rmsg.Month), int(rmsg.Day), int(rmsg.Hour))
				default:
					continue
				}
			}
		}
		l.rlker.Lock()
		delete(l.remotes, r.unixConn.RemoteAddr().String())
		l.rlker.Unlock()
		l.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return unix write")
		//write
		for {
			udata, left := l.nmq.Get(r.notice)
			if udata == nil {
				//this is a notice message
				if left == 0 {
					break
				}
				data := makeHeartMsg()
				if !writefunc(data) {
					break
				}
			} else {
				//this is a log message
				if left == 0 {
					select {
					case l.refresh <- struct{}{}:
					default:
					}
				}
				m := (*msg)(udata)
				data := makeLogMsg(l.c.PeerId, m.content, m.filename, m.line, m.memindex)
				if !writefunc(data) {
					//put message back
					l.nmq.Put(udata)
					break
				}
			}
		}
		l.rlker.Lock()
		delete(l.remotes, r.unixConn.RemoteAddr().String())
		l.rlker.Unlock()
		l.wg.Done()
	}()
}

func (l *Logger) workFile() {
	//defer fmt.Println("return work file")
	var e error
	var n int
	var send int
	for {
		udata, _ := l.fmq.Get(nil)
		if udata == nil {
			break
		}
		data := *(*[]byte)(udata)
		send = 0
		for {
			if n, e = l.locallogfile.Write(data[send:]); e != nil {
				panic(fmt.Sprintf("write log message to locallogfile error:%s\n", e))
			} else if n < len(data) {
				send = n
			} else {
				break
			}
		}
		if l.nmq != nil && l.nmq.Num() == 0 {
			select {
			case l.refresh <- struct{}{}:
			default:
			}
		}
		finfo, _ := l.locallogfile.Stat()
		if finfo.Size()/int64(1024*1024) > l.c.SplitSize {
			logfilename := l.c.LocalDir + "/" + newLogfileName()
			var templogfile *os.File
			var tempfinfo os.FileInfo
			if templogfile, e = os.OpenFile(logfilename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644); e != nil {
				continue
			} else if tempfinfo, e = templogfile.Stat(); e != nil {
				continue
			} else {
				l.finfos = append(l.finfos, tempfinfo)
				go l.locallogfile.Close()
				l.locallogfile = templogfile
			}
		}
	}
	l.wg.Done()
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.write(1, format, v...)
}
func (l *Logger) Info(format string, v ...interface{}) {
	l.write(2, format, v...)
}
func (l *Logger) Warn(format string, v ...interface{}) {
	l.write(3, format, v...)
}
func (l *Logger) Error(format string, v ...interface{}) {
	l.write(4, format, v...)
}
func (l *Logger) Panic(format string, v ...interface{}) {
	l.write(5, format, v...)
	panic("")
}
func (l *Logger) write(lv int32, format string, v ...interface{}) {
	if format[len(format)-1] != '\n' {
		format += "\n"
	}
	content := fmt.Sprintf(format, v...)
	level_prefix := level(lv)
	time_prefix := time.Now().UTC().Format("2006-01-02_15:04:05_MST")
	file_prefix := ""
	if _, filename, line, ok := runtime.Caller(2); ok {
		file_prefix = fmt.Sprintf("%s:%d", filename, line)
	}
	str := fmt.Sprintf("%s %s %s ->> %s", level_prefix, time_prefix, file_prefix, content)
	b := str2byte(str)
	//make local log message
	if l.fmq != nil {
		l.fmq.Put(unsafe.Pointer(&b))
	}
	if l.c.Std {
		fmt.Printf("%s", str)
	}
}
func (l *Logger) Close() {
	l.clker.Lock()
	close(l.closech)
	l.clker.Unlock()
	if l.tcpListener != nil {
		l.tcpListener.Close()
	}
	if l.unixListener != nil {
		l.unixListener.Close()
	}
	if l.fmq != nil {
		l.fmq.Close()
	}
	if l.nmq != nil {
		l.nmq.Close()
	}
	l.wg.Wait()
	l.locallogfile.Close()
	l.netinfofile.Close()
	l.netlogfile.Close()
}
func (l *Logger) Remove(year, month, day, hour int) error {
	l.clker.Lock()
	select {
	case <-l.closech:
		//closing
		l.clker.Unlock()
		return nil
	default:
		l.wg.Add(1)
		defer l.wg.Done()
	}
	l.clker.Unlock()
	if l.nmq == nil && l.fmq == nil {
		//not using local log and net log
		return nil
	}
	t := time.Date(year, time.Month(month), day, hour, 0, 0, 0, time.UTC)
	s := t.Format(timeformat)
	l.flker.Lock()
	defer l.flker.Unlock()
	end := ""
	if l.nmq != nil {
		filename := filepath.Base(l.netlogfile.Name())
		if filename > s {
			end = s
		} else {
			end = filename
		}
	} else {
		filename := filepath.Base(l.locallogfile.Name())
		if filename > s {
			end = s
		} else {
			end = filename
		}
	}
	count := 0
	var e error
	for _, finfo := range l.finfos {
		filename := filepath.Base(finfo.Name())
		if filename >= end {
			break
		}
		if e = os.Remove(l.c.LocalDir + "/" + filename); e != nil {
			if _, e = os.Stat(l.c.LocalDir + "/" + filename); e != nil {
				if os.IsNotExist(e) {
					count++
					continue
				} else {
					l.finfos = l.finfos[count:]
					l.fileindex = l.fileindex - count
					return e
				}
			}
		}
		count++
	}
	l.finfos = l.finfos[count:]
	l.fileindex = 0

	return nil
}
