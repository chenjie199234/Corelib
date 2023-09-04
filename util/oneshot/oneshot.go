package oneshot

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

var lker *sync.Mutex
var pool *sync.Pool
var calls map[string]*call

func init() {
	lker = new(sync.Mutex)
	pool = new(sync.Pool)
	calls = make(map[string]*call, 5)
}

type call struct {
	err   error
	resp  unsafe.Pointer
	wg    sync.WaitGroup
	count int32
}

func getcall() *call {
	c, ok := pool.Get().(*call)
	if !ok {
		c = &call{
			err:  nil,
			resp: nil,
		}
	}
	return c
}
func putcall(c *call) {
	c.err = nil
	c.resp = nil
	c.count = 0
	pool.Put(c)
}
func Do(key string, f func() (unsafe.Pointer, error)) (resp unsafe.Pointer, e error) {
	lker.Lock()
	c, ok := calls[key]
	if !ok {
		c = getcall()
		c.count++
		calls[key] = c
		c.wg.Add(1)
		lker.Unlock()
		c.resp, c.err = f()
		lker.Lock()
		delete(calls, key)
		lker.Unlock()
		c.wg.Done()
	} else {
		c.count++
		lker.Unlock()
	}
	c.wg.Wait()
	if atomic.AddInt32(&c.count, -1) == 0 {
		defer putcall(c)
	}
	return c.resp, c.err
}
