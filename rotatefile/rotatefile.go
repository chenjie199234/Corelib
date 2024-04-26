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

	"github.com/chenjie199234/Corelib/container/list"
	"github.com/chenjie199234/Corelib/pool/bpool"
)

// thread unsafe
type RotateFile struct {
	path, name string
	file       atomic.Value
	buffile    *bufio.Writer
	curlen     int64
	notice     chan struct{}
	caslist    *list.List[[]byte]
	rotate     chan struct{}
	rlker      *sync.Mutex
	rotateret  map[chan error]struct{}
	status     int32 //0 closed,1 working
	closewait  *sync.WaitGroup
}

func NewRotateFile(path, name string) (*RotateFile, error) {
	if name == "" {
		return nil, errors.New("[rotatefile.New] error: name is empty")
	}
	if path == "" {
		return nil, errors.New("[rotatefile.New] error: path is empty")
	}
	//create the first file
	tempfile, e := createfile(path, name)
	if e != nil {
		return nil, e
	}
	rf := &RotateFile{
		path:      path,
		name:      name,
		file:      atomic.Value{},
		buffile:   bufio.NewWriterSize(tempfile, 4096),
		curlen:    0,
		notice:    make(chan struct{}, 1),
		caslist:   list.NewList[[]byte](),
		rotate:    make(chan struct{}, 1),
		rlker:     &sync.Mutex{},
		rotateret: make(map[chan error]struct{}),
		status:    1,
		closewait: &sync.WaitGroup{},
	}
	rf.file.Store(tempfile)
	rf.closewait.Add(1)
	go rf.run()
	return rf, nil
}

func createfile(path string, name string) (*os.File, error) {
	for {
		now := time.Now().UTC()
		filename := fmt.Sprintf("/%s_%s_utc", name, now.Format("2006-01-02_15_04_05.000000000"))
		_, e := os.Lstat(filepath.Clean(path + filename))
		if e == nil {
			//already exist
			continue
		} else if !os.IsNotExist(e) {
			return nil, errors.New("[rotatefile.Create] error: " + e.Error())
		}
		if e := os.MkdirAll(path, 0755); e != nil {
			return nil, errors.New("[rotatefile.Create] error: " + e.Error())
		}
		file, e := os.OpenFile(path+filename, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if e != nil {
			return nil, errors.New("[rotatefile.Create] error: " + e.Error())
		}
		return file, nil
	}
}

func (f *RotateFile) run() {
	defer f.closewait.Done()
	flush := func() {
		if e := f.buffile.Flush(); e != nil {
			fmt.Println("[rotatefile.run] flush error: " + e.Error())
		}
	}
	write := func() bool {
		if tmp, e := f.caslist.Pop(nil); e == nil {
			buf := tmp
			if n, e := f.buffile.Write(buf); e != nil {
				fmt.Println("[rotatefile.run] write error: " + e.Error())
			} else {
				f.curlen += int64(n)
			}
			bpool.Put(&buf)
			return true
		}
		return false
	}
	rotate := func() {
		flush()
		tmpfile, e := createfile(f.path, f.name)
		if e != nil {
			e = errors.New("[rotatefile.Rotate] error: " + e.Error())
			f.rlker.Lock()
			for retch := range f.rotateret {
				retch <- e
			}
			f.rotateret = make(map[chan error]struct{})
			f.rlker.Unlock()
			return
		}
		oldfile := f.file.Load().(*os.File)
		oldfile.Close()
		f.file.Store(tmpfile)
		f.buffile.Reset(tmpfile)
		f.curlen = 0
		f.rlker.Lock()
		for retch := range f.rotateret {
			retch <- nil
		}
		f.rotateret = make(map[chan error]struct{})
		f.rlker.Unlock()
	}
	tker := time.NewTicker(time.Second)
	for {
		select {
		case <-tker.C:
			flush()
		case <-f.rotate:
			rotate()
		default:
			if write() {
				continue
			}
			if atomic.LoadInt32(&f.status) == 0 {
				flush()
				return
			}
			select {
			case <-f.notice:
			case <-f.rotate:
				rotate()
			case <-tker.C:
				flush()
			}
		}
	}
}

func (f *RotateFile) RotateNow() error {
	notice := make(chan error, 1)
	f.rlker.Lock()
	f.rotateret[notice] = struct{}{}
	f.rlker.Unlock()
	select {
	case f.rotate <- struct{}{}:
	default:
	}
	e := <-notice
	return e
}
func (f *RotateFile) CleanNow(lastModTimestampBeforeThisNS int64) error {
	fileinfos, e := os.ReadDir(f.path)
	if e != nil {
		return errors.New("[rotatefile.Clean] error: " + e.Error())
	}
	for _, fileinfo := range fileinfos {
		if fileinfo.IsDir() {
			continue
		}
		filename := fileinfo.Name()
		if len(filename) <= 34 {
			continue
		}
		curfile := f.file.Load().(*os.File)
		curfilename := curfile.Name()
		if filepath.Base(filename) == filepath.Base(curfilename) {
			continue
		}
		if !strings.HasPrefix(filename, f.name+"_") {
			continue
		}
		if filename[len(filename)-4:] != "_utc" {
			continue
		}
		fileCtimeStr := filename[len(f.name)+1 : len(filename)-4]
		if _, e := time.Parse("2006-01-02_15_04_05.000000000", fileCtimeStr); e != nil {
			continue
		}
		info, e := fileinfo.Info()
		if e != nil {
			return errors.New("[rotatefile.Clean] error: " + e.Error())
		}
		if info.ModTime().UnixNano() <= lastModTimestampBeforeThisNS {
			if e := os.Remove(f.path + "/" + filename); e != nil {
				return errors.New("[rotatefile.Clean] error: " + e.Error())
			}
		}
	}
	return nil
}

// return byte num
func (f *RotateFile) GetCurFileLen() int64 {
	return f.curlen
}
func (f *RotateFile) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if atomic.LoadInt32(&f.status) == 0 {
		return 0, fmt.Errorf("[rotatefile.Write] rotate file closed")
	}
	buf := bpool.Get(len(data))
	copy(buf, data)
	f.caslist.Push(buf)
	select {
	case f.notice <- struct{}{}:
	default:
	}
	return len(data), nil
}

func (f *RotateFile) Close() {
	atomic.StoreInt32(&f.status, 0)
	f.closewait.Wait()
	f.file.Load().(*os.File).Close()
}
