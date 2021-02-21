package once

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type SingleCall struct {
	sync.Mutex
	pool  *sync.Pool
	calls map[string]*call
}
type call struct {
	err   error
	resp  unsafe.Pointer
	wg    *sync.WaitGroup
	once  *SingleCall
	count int32
}

func NewSingleCall() *SingleCall {
	return &SingleCall{
		pool:  &sync.Pool{},
		calls: make(map[string]*call),
	}
}
func (this *SingleCall) getcall() *call {
	c, ok := this.pool.Get().(*call)
	if !ok {
		c = &call{
			err:  nil,
			resp: nil,
			wg:   &sync.WaitGroup{},
			once: this,
		}
	}
	return c
}
func (this *SingleCall) putcall(c *call) {
	c.err = nil
	c.resp = nil
	c.wg = nil
	c.once = nil
	c.count = 0
	this.pool.Put(c)
}
func (this *SingleCall) Do(key string, f func() (unsafe.Pointer, error)) (resp unsafe.Pointer, e error) {
	this.Lock()
	c, ok := this.calls[key]
	if !ok {
		c = this.getcall()
		c.count++
		this.calls[key] = c
		c.wg.Add(1)
		this.Unlock()
		c.resp, c.err = f()
		c.once.Lock()
		delete(this.calls, key)
		c.once.Unlock()
		c.wg.Done()
	} else {
		c.count++
		this.Unlock()
	}
	c.wg.Wait()
	if atomic.AddInt32(&c.count, -1) == 0 {
		defer this.putcall(c)
	}
	return c.resp, c.err
}
