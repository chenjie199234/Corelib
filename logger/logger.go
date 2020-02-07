package logger

import (
	"bufio"
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
	PeerId int64 //self id
	//net
	TcpAddr  string
	UnixAddr string
	Timeout  int64 //unit millisecond,default 1000,min 200
	Verify   bool
	//local
	LocalDir  string
	SplitSize int64 //unit M,default 100M,min 20M
	//std
	Std bool
}
type Logger struct {
	c           *LoggerConfig   //config
	wg          *sync.WaitGroup //wait all worker to close
	clker       *sync.Mutex
	closech     chan struct{}
	localclosed bool

	locallogfile *os.File  //local log file
	fmq          *nofullMQ //mem message queue for local log
	flker        *sync.Mutex
	finfos       []os.FileInfo

	netinfofile     *os.File
	netlogfileindex int //the index in 'finfos' for the netlogfile
	netlogfile      *os.File
	netlogreader    *bufio.Reader
	nmq             *nofullMQ

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
	content       []byte
	filename      string //file name
	offset        uint32 //end offset of this msg
	done          bool
	memindex      uint32
	startsynctime int64
}

//first memmq is for local log
//second memmq is for net log
func NewLogger(c *LoggerConfig) *Logger {
	//check config
	if c == nil || (c.TcpAddr == "" && c.UnixAddr == "" && c.LocalDir == "" && !c.Std) {
		panic("logger: config is empty\n")
	}
	//the net log is based on the local log
	if c.TcpAddr != "" || c.UnixAddr != "" {
		if c.LocalDir == "" {
			c.LocalDir = DefaultLocalDir
		}
		if c.Timeout < 200 {
			c.Timeout = DefaultTimeout
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
		instance.fmq = newmq(1024)
		instance.flker = new(sync.Mutex)
		instance.newLocal()
	}
	if c.TcpAddr != "" || c.UnixAddr != "" {
		instance.nmq = newmq(1024)
		instance.refresh = make(chan struct{}, 1)
		instance.update = make(chan struct{}, 1)
		instance.msglist = make([]*msg, DefaultNetLogNum)
		instance.msgheadindex = 0
		instance.msgtailindex = 0
		instance.msgcount = 0
		instance.rlker = new(sync.Mutex)
		instance.remotes = make(map[string]*remote)
		instance.newNetFile()
		instance.newNetConn()
	}
	return instance
}
func (l *Logger) newLocal() {
	//create log dir
	if finfo, e := os.Stat(l.c.LocalDir); e != nil {
		if os.IsNotExist(e) {
			if e := os.Mkdir(l.c.LocalDir, 0755); e != nil {
				panic(fmt.Sprintf("logger: logdir doesn't exist and create it error:%s\n", e))
			}
		} else {
			panic(fmt.Sprintf("logger: get logdir's info error:%s\n", e))
		}
	} else if !finfo.IsDir() {
		panic("logger: logdir's path link to an exist file which is not a dir\n")
	}
	//get all old logfile's info in log dir
	//finfos are sorted by file name
	finfos, e := ioutil.ReadDir(l.c.LocalDir)
	if e != nil {
		panic(fmt.Sprintf("logger: read all files in logdir error:%s\n", e))
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
					panic(fmt.Sprintf("logger: create new logfile error:%s\n"+
						"delete it error:%s\n"+
						"please delete it manually\n", e, ee))
				}
			}
			panic(fmt.Sprintf("create new logfile error:%s\n", e))
		}
		var tempfinfo os.FileInfo
		if tempfinfo, e = l.locallogfile.Stat(); e != nil {
			panic(fmt.Sprintf("logger: get new logfile's fileinfo error:%s\n", e))
		}
		l.finfos = append(l.finfos, tempfinfo)
	} else {
		//use this old logfile
		var e error
		logfilename := l.c.LocalDir + "/" + filepath.Base(l.finfos[len(l.finfos)-1].Name())
		if l.locallogfile, e = os.OpenFile(logfilename, os.O_WRONLY|os.O_APPEND, 0644); e != nil {
			panic(fmt.Sprintf("logger: open old logfile error:%s\n", e))
		}
	}
	l.wg.Add(1)
	go l.workLocal()
}
func (l *Logger) workLocal() {
	//defer fmt.Println("return work file")
	var e error
	var n int
	var send int
	for {
		udata, _ := l.fmq.get(nil)
		if udata == nil {
			//closed
			break
		}
		data := *(*[]byte)(udata)
		send = 0
		for {
			if n, e = l.locallogfile.Write(data[send:]); e != nil {
				panic(fmt.Sprintf("logger: write log message to locallogfile error:%s\n", e))
			} else if n < len(data) {
				send = n
			} else {
				break
			}
		}
		if l.nmq != nil && l.nmq.num == 0 {
			select {
			case l.refresh <- struct{}{}:
			default:
			}
		}
		finfo, _ := l.locallogfile.Stat()
		if finfo != nil && finfo.Size()/int64(1024*1024) > l.c.SplitSize {
			//create new logfile
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
	l.localclosed = true
	l.wg.Done()
}
func (l *Logger) newNetFile() {
	var filename string
	var fileoffset uint32
	//var curline uint32
	tempinfo, e := os.Stat(l.c.LocalDir + "/.netlog")
	if e != nil && !os.IsNotExist(e) {
		panic(fmt.Sprintf("logger: get .netlog fileinfo error:%s\n", e))
	} else if e == nil {
		//there is a .netlog file
		if tempinfo.IsDir() {
			panic("logger: .netlog is a dir\n")
		}
		if l.netinfofile, e = os.OpenFile(l.c.LocalDir+"/.netlog", os.O_RDWR, 0644); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
			}
			panic(fmt.Sprintf("logger: open .netlog file error:%s\n", e))
		}
		//read .netlog
		var d []byte
		if d, e = ioutil.ReadAll(l.netinfofile); e != nil {
			panic(fmt.Sprintf("logger: read .netlog file error:%s\n", e))
		}
		if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
			}
			panic(fmt.Sprintf("logger: seek .netlog file after read error:%s\n", e))
		}
		fileoffset, filename = parseLoginfo(d)
		fmt.Println(fileoffset, filename)
		if filename == "" {
			panic("logger: .netlog file data broken,please fix it manually\n")
		}

		//find the local logfile
		find := false
		for tempindex, finfo := range l.finfos {
			tempfilename := filepath.Base(finfo.Name())
			if tempfilename >= filename {
				if tempfilename != filename {
					filename = tempfilename
					l.netlogfileindex = tempindex
					fileoffset = 0
					var n int
					if n, e = l.netinfofile.Write(serializeLoginfo(fileoffset, filename)); e != nil {
						l.netinfofile.Close()
						if n != 0 {
							panic(fmt.Sprintf("logger: rewrite .netlog file error:%s\n"+
								"data dirty,please fix it manually\n"+
								"current logfile name:%s\n", e, tempfilename))
						}
						panic(fmt.Sprintf("logger: rewrite .netlog file error:%s\n", e))
					}
					if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
						l.netinfofile.Close()
						panic(fmt.Sprintf("logger: seek .netlog file after rewrite error:%s\n", e))
					}
				}
				find = true
				break
			}
		}
		if !find {
			panic("logger: logfile name in .netlog is newer then all the logfiles,this is impossible\n")
		}
	} else {
		//there is no .netlog file
		if l.netinfofile, e = os.OpenFile(l.c.LocalDir+"/.netlog", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644); e != nil {
			if l.netinfofile != nil {
				l.netinfofile.Close()
				if ee := os.Remove(l.c.LocalDir + "/.netlog"); ee != nil {
					panic(fmt.Sprintf("logger: create .netlog file error:%s\n"+
						"delete it error:%s\n"+
						"please delete it manually\n", e, ee))
				}
			}
			panic(fmt.Sprintf("logger: create .netlog file error:%s\n", e))
		}
		filename = filepath.Base(l.finfos[0].Name())
		l.netlogfileindex = 0
		fileoffset = 0
		if _, e := l.netinfofile.Write(serializeLoginfo(fileoffset, filename)); e != nil {
			l.netinfofile.Close()
			if ee := os.Remove(l.c.LocalDir + "/.netlog"); ee != nil {
				panic(fmt.Sprintf("logger: init .netlog file error:%s\n"+
					"delete it error:%s\n"+
					"please delete it manually\n", e, ee))
			}
			panic(fmt.Sprintf("logger: init .netlog file error:%s\n", e))
		}
		if _, e = l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
			l.netinfofile.Close()
			panic(fmt.Sprintf("logger: seek .netlog file after init error:%s\n", e))
		}
	}
	//open logfile for net read
	if l.netlogfile, e = os.Open(l.c.LocalDir + "/" + filename); e != nil {
		if l.netlogfile != nil {
			l.netlogfile.Close()
		}
		panic(fmt.Sprintf("logger: open logfile for net read error:%s\n", e))
	}
	if fileoffset > 0 {
		if _, e = l.netlogfile.Seek(int64(fileoffset), os.SEEK_SET); e != nil {
			l.netlogfile.Close()
			panic(fmt.Sprintf("logger: seek logfile before net read error:%s\n", e))
		}
	}
	l.netlogreader = bufio.NewReader(l.netlogfile)

	go l.workNetFile(filename, fileoffset)
}
func (l *Logger) newNetConn() {
	//check heart
	go func() {
		tker := time.NewTicker(time.Duration(l.c.Timeout/3) * time.Millisecond)
		tkercount := 0
		for {
			<-tker.C
			tkercount++
			now := time.Now().UnixNano()
			l.rlker.Lock()
			for _, r := range l.remotes {
				//send heartbeat
				select {
				case r.notice <- 1:
				default:
				}
				//check timeout
				if tkercount >= 3 {
					if now-atomic.LoadInt64(&r.lastHeart) > l.c.Timeout*1000*1000 {
						switch r.netkind {
						case "tcp":
							r.tcpConn.Close()
						case "unix":
							r.unixConn.Close()
						}
					}
				}
			}
			if tkercount >= 3 {
				tkercount = 0
			}
			l.rlker.Unlock()
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
				r.netkind = "tcp"
				r.tcpConn = c
				r.notice = make(chan uint, 1)
				r.lastHeart = time.Now().UnixNano()
				l.rlker.Lock()
				l.remotes[c.RemoteAddr().String()] = r
				l.rlker.Unlock()
				l.wg.Add(2)
				go l.workNetConn(r)
			}
			l.wg.Done()
		}()
	}
	if l.c.UnixAddr != "" {
		//deal unix net connect
		/*
			if finfo, e := os.Stat(l.c.UnixAddr); e != nil && !os.IsNotExist(e) {
				panic(fmt.Sprintf("unix addr error:%s\n", e))
			} else if e == nil {
				//exist,check the file type
				if finfo.Mode()&os.ModeSocket == 0 {
					panic("unix addr already exists,and it is a normal file or dir")
				}
				if e = os.RemoveAll(l.c.UnixAddr); e != nil {
					panic(fmt.Sprintf("unix addr already exists,and delete it failed:%s\n", e))
				}
			} else {
				//not exist,this is correct
			}
		*/
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
				r.netkind = "unix"
				r.unixConn = c
				r.notice = make(chan uint, 1)
				r.lastHeart = time.Now().UnixNano()
				l.rlker.Lock()
				l.remotes[c.RemoteAddr().String()] = r
				l.rlker.Unlock()
				l.wg.Add(2)
				go l.workNetConn(r)
			}
			l.wg.Done()
		}()
	}
}
func parseLoginfo(d []byte) (uint32, string) {
	if len(d) != 12 {
		return 0, ""
	}
	return binary.BigEndian.Uint32(d[:4]), time.Unix(int64(binary.BigEndian.Uint64(d[4:12])), 0).UTC().Format(timeformat)
}
func serializeLoginfo(fileoffset uint32, filename string) []byte {
	offsetb := make([]byte, 4)
	timeb := make([]byte, 8)
	binary.BigEndian.PutUint32(offsetb, fileoffset)
	t, _ := time.Parse(timeformat, filename)
	binary.BigEndian.PutUint64(timeb, uint64(t.Unix()))
	return append(offsetb, timeb...)
}
func (l *Logger) workNetFile(filename string, fileoffset uint32) {
	//deal net log file
	l.refresh <- struct{}{}
	tmer := time.NewTimer(500 * time.Millisecond)
	updateretry := 0
	updateMSGfunc := func() {
		if l.msgcount <= 0 {
			select {
			case l.refresh <- struct{}{}:
			default:
			}
			return
		}
		//check finished
		temphead := l.msgheadindex
		tempmsgcount := l.msgcount
		for tempmsgcount > 0 && l.msglist[temphead].done {
			tempmsgcount--
			temphead++
			if temphead >= DefaultNetLogNum {
				temphead = 0
			}
		}
		//deal finished
		if tempmsgcount != l.msgcount {
			var newfileoffset uint32
			var newfilename string
			if temphead == l.msgtailindex {
				//finish all net log in buf
				var tempindex uint32
				if temphead == 0 {
					tempindex = DefaultNetLogNum - 1
				} else {
					tempindex = temphead - 1
				}
				newfilename = l.msglist[tempindex].filename
				newfileoffset = l.msglist[tempindex].offset + uint32(len(l.msglist[tempindex].content)) + 1 //add 1 for '\n'
			} else {
				newfilename = l.msglist[temphead].filename
				newfileoffset = l.msglist[temphead].offset
			}
			if _, e := l.netinfofile.Seek(0, os.SEEK_SET); e != nil {
				updateretry++
				if updateretry > 3 {
					panic(fmt.Sprintf("seek before rewrite .netlog error:%s\n", e))
				}
			} else if n, e := l.netinfofile.Write(serializeLoginfo(newfileoffset, newfilename)); e != nil {
				//file data is dirty,must panic now
				if n != 0 {
					if l.netinfofile != nil {
						l.netinfofile.Close()
					}
					panic(fmt.Sprintf("rewrite .netlog error:%s\n"+
						"data dirty,please fix it manully\n"+
						"current logfile name:%s\n", e, newfilename))
				}
				//file data wasn't dirty,give a chance to retry
				updateretry++
				if updateretry > 3 {
					if l.netinfofile != nil {
						l.netinfofile.Close()
					}
					panic(fmt.Sprintf("rewrite .netlog error:%s\n", e))
				}
			} else {
				updateretry = 0
				l.msgcount = tempmsgcount
				l.msgheadindex = temphead
				select {
				case l.refresh <- struct{}{}:
				default:
				}
			}
		}
		//check the rest
		now := time.Now().UnixNano()
		temphead = l.msgheadindex
		l.rlker.Lock()
		for l.msgcount > 0 && len(l.remotes) > 0 {
			if !l.msglist[temphead].done && (now-l.msglist[temphead].startsynctime) > 500*1000*1000 {
				l.nmq.put(unsafe.Pointer(l.msglist[temphead]))
			} else {
				break
			}
			temphead++
			if temphead == l.msgtailindex {
				break
			}
		}
		l.rlker.Unlock()
	}
	refreshretry := false
	content := make([]byte, 1024)
	contentOffset := uint32(0)
	refreshMSGfunc := func() {
		refreshretry = false
		for {
			if b, e := l.netlogreader.ReadByte(); e != nil {
				if e != io.EOF {
					panic(fmt.Sprintf("read logfile for net log error:%s\n", e))
				}
				locfilename := filepath.Base(l.locallogfile.Name())
				if filename != locfilename {
					//local log file changed to a new file
					if contentOffset != 0 {
						if !refreshretry {
							refreshretry = true
							continue
						}
						panic(fmt.Sprintf("unexpected EOF on logfile:%s\n", filename))
					}
					refreshretry = false
					//this logfile complete send,search next
					l.flker.Lock()
					l.netlogfileindex++
					filename = filepath.Base(l.finfos[l.netlogfileindex].Name())
					fileoffset = 0
					var tempfile *os.File
					if tempfile, e = os.Open(l.c.LocalDir + "/" + filename); e != nil {
						if tempfile != nil {
							tempfile.Close()
						}
						panic(fmt.Sprintf("open logfile for net read error:%s\n", e))
					}
					go l.netlogfile.Close()
					l.netlogfile = tempfile
					l.netlogreader = bufio.NewReader(l.netlogfile)
					l.flker.Unlock()
					if l.msgcount < DefaultNetLogNum {
						continue
					}
				}
				break
			} else if b != '\n' {
				if contentOffset < uint32(len(content)) {
					content[contentOffset] = b
				} else {
					content = append(content, make([]byte, 1024)...)
					content[contentOffset] = b
				}
				contentOffset++
			} else {
				message := &msg{
					content:       content[:contentOffset],
					filename:      filename,
					offset:        fileoffset,
					done:          false,
					memindex:      l.msgtailindex,
					startsynctime: time.Now().UnixNano(),
				}
				l.msglist[l.msgtailindex] = message
				l.msgcount++
				l.msgtailindex++
				if l.msgtailindex >= DefaultNetLogNum {
					l.msgtailindex = 0
				}
				fileoffset += (contentOffset + 1) //add 1 for '\n'
				contentOffset = 0
				l.nmq.put(unsafe.Pointer(message))
				if l.msgcount >= DefaultNetLogNum {
					content = content[:1024]
					break
				}
			}
		}
	}
	for {
		select {
		case <-tmer.C:
			updateMSGfunc()
			tmer.Reset(500 * time.Millisecond)
			if len(tmer.C) > 0 {
				<-tmer.C
			}
		case <-l.update:
			updateMSGfunc()
			tmer.Reset(500 * time.Millisecond)
			if len(tmer.C) > 0 {
				<-tmer.C
			}
		case <-l.refresh:
			refreshMSGfunc()
			l.rlker.Lock()
			if l.localclosed && (l.msgcount == 0 || len(l.remotes) == 0) {
				select {
				case <-l.closech:
					l.nmq.close()
					return
				default:
				}
			}
			l.rlker.Unlock()
		}
	}
}
func (l *Logger) workNetConn(r *remote) {
	var ew error
	var nw int
	var sendnum int
	writefunc := func(data []byte) bool {
		sendnum = 0
		for {
			switch r.netkind {
			case "tcp":
				if nw, ew = r.tcpConn.Write(data[sendnum:]); ew != nil {
					r.tcpConn.Close()
					return false
				}
			case "unix":
				if nw, ew = r.unixConn.Write(data[sendnum:]); ew != nil {
					r.unixConn.Close()
					return false
				}
			default:
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
				case CONFIRM:
					cmsg := msg.GetConfirm()
					message := l.msglist[cmsg.Memindex]
					if message.filename == cmsg.Filename && message.offset == cmsg.Offset && message.startsynctime == cmsg.Synctime {
						l.msglist[cmsg.Memindex].done = true
						select {
						case l.update <- struct{}{}:
						default:
						}
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
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			delete(l.remotes, r.tcpConn.RemoteAddr().String())
		case "unix":
			r.unixConn.Close()
			delete(l.remotes, r.unixConn.RemoteAddr().String())
		}
		l.rlker.Unlock()
		l.wg.Done()
	}()
	go func() {
		//defer fmt.Println("return tcp write")
		//write
		for {
			udata, notice := l.nmq.get(r.notice)
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
				//this is a log message
				m := (*msg)(udata)
				data := makeLogMsg(l.c.PeerId, m.content, m.filename, m.offset, m.memindex, m.startsynctime)
				if !writefunc(data) {
					//put message back
					l.nmq.put(udata)
					break
				}
			}
		}
		l.rlker.Lock()
		switch r.netkind {
		case "tcp":
			r.tcpConn.Close()
			delete(l.remotes, r.tcpConn.RemoteAddr().String())
		case "unix":
			r.unixConn.Close()
			delete(l.remotes, r.unixConn.RemoteAddr().String())
		}
		l.rlker.Unlock()
		l.wg.Done()
	}()
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
		l.fmq.put(unsafe.Pointer(&b))
	}
	if l.c.Std {
		fmt.Printf("%s", str)
	}
}
func (l *Logger) Close() {
	if l.tcpListener != nil {
		l.tcpListener.Close()
	}
	if l.unixListener != nil {
		l.unixListener.Close()
	}
	if l.fmq != nil {
		l.fmq.close()
	}
	l.clker.Lock()
	close(l.closech)
	l.clker.Unlock()
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
			//remove failed
			if _, e = os.Stat(l.c.LocalDir + "/" + filename); e != nil && os.IsNotExist(e) {
				//already deleted,continue
				count++
				continue
			}
			l.finfos = l.finfos[count:]
			if l.nmq != nil {
				l.netlogfileindex = l.netlogfileindex - count
			}
			return e
		}
		//remove success
		count++
	}
	l.finfos = l.finfos[count:]
	l.netlogfileindex = 0

	return nil
}
