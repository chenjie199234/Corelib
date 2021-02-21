package rotatefile

import (
	"bufio"
	"fmt"
	"io/ioutil"
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

type RotateCap uint //unit M
type RotateTime uint
type KeepDays uint //unit Day

const (
	RotateOff RotateTime = iota + 1
	RotateHour
	RotateDay
	RotateMonth
)

//thread unsafe
type RotateFile struct {
	path      string
	name      string
	ext       string
	file      *os.File
	buffile   *bufio.Writer
	maxcap    uint64
	curcap    uint64
	cycle     RotateTime
	keep      uint64
	lastcycle *time.Time

	sync.Mutex
	head *node
	tail *node
	pool *sync.Pool

	status int32 //0 closed,1 working
	waitch chan struct{}
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
func NewRotateFile(path, name, ext string, maxcap RotateCap, cycle RotateTime, maxdays KeepDays) (*RotateFile, error) {
	//create the first file
	if path == "" {
		//default current dir
		path = "./"
	}
	tempfile, now, e := createfile(path, name, ext)
	if e != nil {
		return nil, e
	}
	rf := &RotateFile{
		path:      path,
		name:      name,
		ext:       ext,
		file:      tempfile,
		buffile:   bufio.NewWriterSize(tempfile, 4096),
		maxcap:    uint64(maxcap) * 1024 * 1024,
		curcap:    0,
		cycle:     cycle,
		keep:      uint64(maxdays),
		lastcycle: now,

		pool: &sync.Pool{},

		status: 1,
		waitch: make(chan struct{}, 1),
	}
	tempnode := rf.getnode()
	rf.head = tempnode
	rf.tail = tempnode
	go rf.run()
	return rf, nil
}
func createfile(path string, name string, ext string) (*os.File, *time.Time, error) {
	for {
		now := time.Now()
		now = now.UTC()
		var filename string
		if name != "" {
			filename = fmt.Sprintf("/%s_%s", now.Format("2006-01-02_15:04:05:000000000"), name)
		} else {
			filename = fmt.Sprintf("/%s", now.Format("2006-01-02_15:04:05:000000000"))
		}
		if ext != "" {
			if ext[0] != '.' {
				filename += "."
			}
			filename += ext
		}
		_, e := os.Stat(path + filename)
		if e == nil {
			continue
		} else if !os.IsNotExist(e) {
			return nil, nil, fmt.Errorf("[rotate file]create rotate file error:%s", e)
		}
		if e := os.MkdirAll(path, 0755); e != nil {
			return nil, nil, fmt.Errorf("[rotate file]create rotate file error:%s", e)
		}
		file, e := os.OpenFile(path+filename, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if e != nil {
			return nil, nil, fmt.Errorf("[rotate file]create rotate file error:%s", e)
		}
		return file, &now, nil
	}
}

func (f *RotateFile) run() {
	f.runcleaner()
	tker := time.NewTicker(time.Second)
	cleantker := time.NewTicker(24 * time.Hour)
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
					fmt.Printf("[rotate file]flush disk error:%s\n", e)
				}
				f.waitch <- struct{}{}
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
	if f.keep == 0 {
		return
	}
	now := time.Now()
	now = now.UTC()
	finfos, e := ioutil.ReadDir(f.path)
	if e != nil {
		fmt.Printf("[rotate file] read file dir error:%s\n", e)
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
		if f.name != "" {
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
			if !strings.HasPrefix(reststr, f.name) {
				continue
			}
			extstr := reststr[len(f.name):]
			if f.ext == "" {
				if extstr != "" {
					continue
				}
			} else {
				if extstr == "" {
					continue
				}
				if extstr[0] != '.' {
					continue
				}
				if extstr != f.ext && extstr[1:] != f.ext {
					continue
				}
			}
		} else {
			index := strings.Index(filename, ".")
			if index == -1 {
				continue
			}
			timestr = filename[:index]
			extstr := filename[index:]
			if f.ext == "" {
				if extstr != "" {
					continue
				}
			} else {
				if extstr == "" {
					continue
				}
				if extstr != f.ext && extstr[1:] != f.ext {
					continue
				}
			}
		}
		t, e := time.Parse("2006-01-02_15:04:05:000000000", timestr)
		if e != nil {
			continue
		}
		if now.Sub(t) >= time.Duration(f.keep)*24*time.Hour {
			if e = os.Remove(f.path + "/" + filename); e != nil {
				fmt.Printf("[rotate file] clean old file error:%s\n", e)
			}
		}
	}
}
func (f *RotateFile) runticker() {
	if e := f.buffile.Flush(); e != nil {
		fmt.Printf("[rotate file]flush disk error:%s\n", e)
		return
	}
	now := time.Now()
	now = now.UTC()
	need := false
	switch f.cycle {
	case RotateHour:
		if f.lastcycle.Hour() != now.Hour() {
			need = true
		}
	case RotateDay:
		if f.lastcycle.Day() != now.Day() {
			need = true
		}
	case RotateMonth:
		if f.lastcycle.Month() != now.Month() {
			need = true
		}
	}
	need = true
	if need {
		tempfile, now, e := createfile(f.path, f.name, f.ext)
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
				fmt.Printf("[rotate file]flush disk error:%s\n", e)
				bufpool.PutBuffer(data)
				return
			}
			tempfile, now, e := createfile(f.path, f.name, f.ext)
			if e != nil {
				fmt.Println(e)
				bufpool.PutBuffer(data)
				return
			}
			f.file.Close()
			f.file = tempfile
			f.buffile.Reset(f.file)
			f.curcap = 0
			switch f.cycle {
			case RotateHour:
				if f.lastcycle.Hour() != now.Hour() {
					f.lastcycle = now
				}
			case RotateDay:
				if f.lastcycle.Day() != now.Day() {
					f.lastcycle = now
				}
			case RotateMonth:
				if f.lastcycle.Month() != now.Month() {
					f.lastcycle = now
				}
			}
		}
		if n, e := f.buffile.Write(data.Bytes()); e != nil {
			f.curcap += uint64(n)
			fmt.Printf("[rotate file]write disk error:%s\n", e)
		} else {
			f.curcap += uint64(data.Len())
		}
	} else if _, e := f.buffile.Write(data.Bytes()); e != nil {
		fmt.Printf("[rotate file]write disk error:%s\n", e)
	}
	bufpool.PutBuffer(data)
}

func (f *RotateFile) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if f.maxcap != 0 && uint64(len(data)) > f.maxcap {
		return 0, fmt.Errorf("[rotate file]data too large")
	}
	if atomic.LoadInt32(&f.status) == 0 {
		return 0, fmt.Errorf("[rotate file]rotate file had been closed")
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
		return 0, fmt.Errorf("[rotate file]data too large")
	}
	if atomic.LoadInt32(&f.status) == 0 {
		return 0, fmt.Errorf("[rotate file]rotate file had been closed")
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
	if atomic.SwapInt32(&f.status, 0) == 0 {
		return
	}
	<-f.waitch
}
