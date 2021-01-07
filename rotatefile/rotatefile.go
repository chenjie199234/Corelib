package rotatefile

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

//thread unsafe
type RotateFile struct {
	path    string
	name    string
	file    *os.File
	buffile *bufio.Writer
	maxcap  uint64
	curcap  uint64

	head *node
	tail *node
	pool *sync.Pool

	status int //0 closed,1 working
	waitch chan struct{}
}
type node struct {
	ch   chan []byte
	next *node
}

func NewRotateFile(path, name string, maxcap uint64) (*RotateFile, error) {
	//create the first file
	tempfile, tempbuffile, e := createfile(path, name)
	if e != nil {
		return nil, e
	}
	tempnode := &node{
		ch:   make(chan []byte, 65536),
		next: nil,
	}
	rf := &RotateFile{
		path:    path,
		name:    name,
		file:    tempfile,
		buffile: tempbuffile,
		maxcap:  maxcap,
		curcap:  0,
		head:    tempnode,
		tail:    tempnode,
		pool:    &sync.Pool{},
		status:  1,
		waitch:  make(chan struct{}, 1),
	}
	go rf.run()
	return rf, nil
}
func createfile(path string, name string) (*os.File, *bufio.Writer, error) {
	for {
		now := time.Now()
		filename := fmt.Sprintf("/%s_%s_%d.log", name, now.Format("2006-01-02_15:04:05"), now.Nanosecond())
		_, e := os.Stat(path + filename)
		if e == nil {
			continue
		} else if !os.IsNotExist(e) {
			return nil, nil, fmt.Errorf("create rotate file error:%s", e)
		}
		file, e := os.OpenFile(path+filename, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if e != nil {
			return nil, nil, fmt.Errorf("create rotate file error:%s", e)
		}
		return file, bufio.NewWriterSize(file, 4096), nil
	}
}
func (f *RotateFile) getnode() *node {
	n, ok := f.pool.Get().(*node)
	if !ok {
		return &node{ch: make(chan []byte, 65536), next: nil}
	}
	return n
}
func (f *RotateFile) putnode(n *node) {
	for len(n.ch) > 0 {
		<-n.ch
	}
	n.next = nil
	f.pool.Put(n)
}
func (f *RotateFile) run() {
	tker := time.NewTicker(time.Second)
	for {
		select {
		case <-tker.C:
			if e := f.buffile.Flush(); e != nil {
				fmt.Printf("[rotate file]flush disk error:%s\n", e)
			}
		case data, ok := <-f.head.ch:
			if !ok {
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
			if f.curcap+uint64(len(data)) > f.maxcap {
				if e := f.buffile.Flush(); e != nil {
					fmt.Printf("[rotate file]flush disk error:%s\n", e)
					continue
				}
				f.file.Close()
				tempfile, tempbuffile, e := createfile(f.path, f.name)
				if e != nil {
					fmt.Printf("[rotate file]create new file error:%s\n", e)
					continue
				}
				f.file = tempfile
				f.buffile = tempbuffile
				f.curcap = 0
			}
			if n, e := f.buffile.Write(data); e != nil {
				f.curcap += uint64(n)
				fmt.Printf("[rotate file]write error:%s\n", e)
				continue
			}
			f.curcap += uint64(len(data))
		default:
			if f.head.next != nil && len(f.head.ch) == 0 {
				f.head = f.head.next
			}
			select {
			case <-tker.C:
				if e := f.buffile.Flush(); e != nil {
					fmt.Printf("[rotate file]flush disk error:%s\n", e)
				}
			case data := <-f.head.ch:
				if f.curcap+uint64(len(data)) > f.maxcap {
					if e := f.buffile.Flush(); e != nil {
						fmt.Printf("[rotate file]flush disk error:%s\n", e)
						continue
					}
					f.file.Close()
					tempfile, tempbuffile, e := createfile(f.path, f.name)
					if e != nil {
						fmt.Printf("[rotate file]create new file error:%s\n", e)
						continue
					}
					f.file = tempfile
					f.buffile = tempbuffile
					f.curcap = 0
				}
				if n, e := f.buffile.Write(data); e != nil {
					f.curcap += uint64(n)
					fmt.Printf("[rotate file]write error:%s\n", e)
					continue
				}
				f.curcap += uint64(len(data))
			}
		}
	}
}

//thread unsafe
func (f *RotateFile) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if f.status == 0 {
		return fmt.Errorf("this rotate file is closed")
	}
	select {
	case f.tail.ch <- data:
	default:
		newtail := f.getnode()
		newtail.ch <- data
		f.tail.next = newtail
		f.tail = newtail
	}
	return nil
}

//thread unsafe
func (f *RotateFile) Close(wait bool) {
	if f.status == 0 {
		return
	}
	f.status = 0
	close(f.tail.ch)
	if wait {
		<-f.waitch
	}
}
