package rotatefile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/util/common"
)

//thread unsafe
type RotateFile struct {
	c         *Config
	file      *os.File
	buffile   *bufio.Writer
	maxcap    uint64
	curcap    uint64
	lastcycle *time.Time

	sync.Mutex
	head *node
	tail *node
	pool *sync.Pool

	status  int32 //0 closed,1 working
	closech chan struct{}
}
type Config struct {
	Path        string
	Name        string
	RotateCap   uint //unit M
	RotateCycle uint //0-off,1-hour,2-day,3-week,4-month
	KeepDays    uint //unit day
}

type node struct {
	ch   chan *bufpool.Buffer
	next *node
}

func (f *RotateFile) getnode() *node {
	n, ok := f.pool.Get().(*node)
	if !ok {
		return &node{ch: make(chan *bufpool.Buffer, 4096), next: nil}
	}
	n.next = nil
	return n
}
func (f *RotateFile) putnode(n *node) {
	for len(n.ch) > 0 {
		<-n.ch
	}
	f.pool.Put(n)
}

//rotatecap unit M
//rotatecycle 0-off,1-hour,2-day,3-week,4-month
//keeydays unit day
func NewRotateFile(c *Config) (*RotateFile, error) {
	if c.Name == "" {
		return nil, errors.New("[rotate file] init error:name is empty")
	}
	if c.Path == "" {
		//default current dir
		c.Path = "./"
	}
	//create the first file
	tempfile, now, e := createfile(c.Path, c.Name)
	if e != nil {
		return nil, e
	}
	rf := &RotateFile{
		c:         c,
		file:      tempfile,
		buffile:   bufio.NewWriterSize(tempfile, 4096),
		maxcap:    uint64(c.RotateCap) * 1024 * 1024,
		curcap:    0,
		lastcycle: now,

		pool: &sync.Pool{},

		status:  1,
		closech: make(chan struct{}),
	}
	tempnode := rf.getnode()
	rf.head = tempnode
	rf.tail = tempnode
	go rf.run()
	return rf, nil
}
func createfile(path string, name string) (*os.File, *time.Time, error) {
	for {
		now := time.Now()
		now = now.UTC()
		var filename string
		if name != "" {
			filename = fmt.Sprintf("/%s_%s", now.Format("2006-01-02_15:04:05.000000000"), name)
		} else {
			filename = fmt.Sprintf("/%s", now.Format("2006-01-02_15:04:05.000000000"))
		}
		_, e := os.Stat(path + filename)
		if e == nil {
			continue
		} else if !os.IsNotExist(e) {
			return nil, nil, errors.New("[rotate file] create rotate file error:" + e.Error())
		}
		if e := os.MkdirAll(path, 0755); e != nil {
			return nil, nil, errors.New("[rotate file] create rotate file error:" + e.Error())
		}
		file, e := os.OpenFile(path+filename, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if e != nil {
			return nil, nil, errors.New("[rotate file] create rotate file error:" + e.Error())
		}
		return file, &now, nil
	}
}

func (f *RotateFile) run() {
	defer close(f.closech)
	f.runcleaner()
	tker := time.NewTicker(time.Second)
	cleantker := time.NewTicker(time.Hour)
	for {
		select {
		case <-tker.C:
			f.runticker()
		case <-cleantker.C:
			f.runcleaner()
		case data := <-f.head.ch:
			f.runwriter(data)
		default:
			if f.head.next != nil && len(f.head.ch) == 0 {
				temp := f.head
				f.head = f.head.next
				f.putnode(temp)
			} else if atomic.LoadInt32(&f.status) == 0 {
				tker.Stop()
				for len(tker.C) > 0 {
					<-tker.C
				}
				if e := f.buffile.Flush(); e != nil {
					fmt.Println("[rotatefile] flush disk error:" + e.Error())
				}
				return
			}
			select {
			case <-tker.C:
				f.runticker()
			case <-cleantker.C:
				f.runcleaner()
			case data := <-f.head.ch:
				f.runwriter(data)
			}
		}
	}
}
func (f *RotateFile) runcleaner() {
	if f.c.KeepDays == 0 {
		return
	}
	now := time.Now()
	now = now.UTC()
	finfos, e := os.ReadDir(f.c.Path)
	if e != nil {
		fmt.Println("[rotatefile.cleaner] read file dir error:" + e.Error())
		return
	}
	for _, finfo := range finfos {
		if finfo.IsDir() {
			continue
		}
		tempfile := (*os.File)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer((&f.file)))))
		if filepath.Base(finfo.Name()) == filepath.Base(tempfile.Name()) {
			continue
		}
		filename := filepath.Base(finfo.Name())
		var timestr string
		index1 := strings.Index(filename, "_")
		if index1 == -1 {
			continue
		}
		index2 := strings.Index(filename[index1+1:], "_")
		if index2 == -1 {
			continue
		}
		timestr = filename[:index1+index2+1]
		reststr := filename[index1+index2+2:]
		if reststr != f.c.Name {
			continue
		}
		t, e := time.Parse("2006-01-02_15:04:05.000000000", timestr)
		if e != nil {
			continue
		}
		if now.Sub(t) >= time.Duration(f.c.KeepDays)*24*time.Hour {
			if e = os.Remove(f.c.Path + "/" + filename); e != nil {
				fmt.Println("[rotatefile.cleaner] clean old file error:" + e.Error())
			}
		}
	}
}
func (f *RotateFile) runticker() {
	if e := f.buffile.Flush(); e != nil {
		fmt.Println("[rotatefile.ticker] flush disk error:" + e.Error())
		return
	}
	now := time.Now()
	now = now.UTC()
	need := false
	switch f.c.RotateCycle {
	case 1:
		if f.lastcycle.Hour() != now.Hour() {
			need = true
		}
	case 2:
		if f.lastcycle.Day() != now.Day() {
			need = true
		}
	case 3:
		if f.lastcycle.Weekday() == now.Weekday() && f.lastcycle.Day() != now.Day() {
			need = true
		}
	case 4:
		if f.lastcycle.Month() != now.Month() {
			need = true
		}
	}
	if need {
		tempfile, now, e := createfile(f.c.Path, f.c.Name)
		if e != nil {
			fmt.Println(e)
			return
		}
		f.file.Close()
		f.file = tempfile
		f.buffile.Reset(f.file)
		f.lastcycle = now
	}
}
func (f *RotateFile) runwriter(data *bufpool.Buffer) {
	if f.maxcap > 0 {
		if f.curcap+uint64(data.Len()) > f.maxcap {
			if e := f.buffile.Flush(); e != nil {
				fmt.Println("[rotatefile.writer] flush disk error:" + e.Error())
				bufpool.PutBuffer(data)
				return
			}
			tempfile, now, e := createfile(f.c.Path, f.c.Name)
			if e != nil {
				fmt.Println(e)
				bufpool.PutBuffer(data)
				return
			}
			f.file.Close()
			f.file = tempfile
			f.buffile.Reset(f.file)
			f.curcap = 0
			switch f.c.RotateCycle {
			case 1:
				if f.lastcycle.Hour() != now.Hour() {
					f.lastcycle = now
				}
			case 2:
				if f.lastcycle.Day() != now.Day() {
					f.lastcycle = now
				}
			case 3:
				if f.lastcycle.Weekday() == now.Weekday() && f.lastcycle.Day() != now.Day() {
					f.lastcycle = now
				}
			case 4:
				if f.lastcycle.Month() != now.Month() {
					f.lastcycle = now
				}
			}
		}
		if n, e := f.buffile.Write(data.Bytes()); e != nil {
			f.curcap += uint64(n)
			fmt.Println("[rotatefile.writer] write disk error:" + e.Error())
		} else {
			f.curcap += uint64(data.Len())
		}
	} else if _, e := f.buffile.Write(data.Bytes()); e != nil {
		fmt.Printf("[rotatefile.writer] write disk error:" + e.Error())
	}
	bufpool.PutBuffer(data)
}

func (f *RotateFile) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if f.maxcap != 0 && uint64(len(data)) > f.maxcap {
		return 0, fmt.Errorf("[rotatefile.Write] data too large")
	}
	if atomic.LoadInt32(&f.status) == 0 {
		return 0, fmt.Errorf("[rotatefile.Write] rotate file had been closed")
	}
	buf := bufpool.GetBuffer()
	buf.Append(common.Byte2str(data))
	f.Lock()
	select {
	case f.tail.ch <- buf:
	default:
		temptail := f.getnode()
		temptail.ch <- buf
		f.tail.next = temptail
		f.tail = temptail
	}
	f.Unlock()
	return len(data), nil
}
func (f *RotateFile) WriteBuf(data *bufpool.Buffer) (int, error) {
	if data.Len() == 0 {
		return 0, nil
	}
	if f.maxcap != 0 && uint64(data.Len()) > f.maxcap {
		return 0, fmt.Errorf("[rotatefile.WriteBuf] data too large")
	}
	if atomic.LoadInt32(&f.status) == 0 {
		return 0, fmt.Errorf("[rotatefile.WriteBuf] rotate file had been closed")
	}
	f.Lock()
	select {
	case f.tail.ch <- data:
	default:
		temptail := f.getnode()
		temptail.ch <- data
		f.tail.next = temptail
		f.tail = temptail
	}
	f.Unlock()
	return data.Len(), nil
}

func (f *RotateFile) Close() {
	atomic.SwapInt32(&f.status, 0)
	<-f.closech
}
