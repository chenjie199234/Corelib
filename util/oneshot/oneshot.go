package oneshot

import (
	"encoding/base64"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

var lker *sync.Mutex
var pool *sync.Pool
var calls map[string]*call

func init() {
	lker = new(sync.Mutex)
	pool = &sync.Pool{New: func() any { return &call{} }}
	calls = make(map[string]*call, 5)
}

type call struct {
	err   error
	resp  unsafe.Pointer
	wg    sync.WaitGroup
	count int32
}

func Do(key string, f func() (unsafe.Pointer, error)) (resp unsafe.Pointer, err error) {
	lker.Lock()
	c, ok := calls[key]
	if !ok {
		c = pool.Get().(*call)
		c.count++
		calls[key] = c
		c.wg.Add(1)
		lker.Unlock()
		func() {
			defer func() {
				if e := recover(); e != nil {
					stack := make([]byte, 2048)
					n := runtime.Stack(stack, false)
					c.err = fmt.Errorf("panic: %v,stack: %s", e, base64.StdEncoding.EncodeToString(stack[:n]))
				}
			}()
			c.resp, c.err = f()
		}()
		c.wg.Done()
	} else {
		for {
			oldcount := atomic.LoadInt32(&c.count)
			if oldcount == 0 {
				resp = c.resp
				err = c.err
				lker.Unlock()
				return
			}
			if !atomic.CompareAndSwapInt32(&c.count, oldcount, oldcount+1) {
				continue
			}
			break
		}
		lker.Unlock()
	}
	c.wg.Wait()
	resp = c.resp
	err = c.err
	if atomic.AddInt32(&c.count, -1) == 0 {
		//the last caller delete the key
		lker.Lock()
		delete(calls, key)
		lker.Unlock()
		//the last caller return the call to the pool
		c.err = nil
		c.resp = nil
		pool.Put(c)
	}
	return
}
