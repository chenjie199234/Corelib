package memqueue

import (
	"sync"
	"unsafe"
)

type MQ struct {
	chanlen     int
	out         chan unsafe.Pointer
	buffer      []unsafe.Pointer
	closestatus bool
	sync.Mutex
}

func New(mincap int) *MQ {
	instance := new(MQ)
	if mincap <= 0 {
		mincap = 128
	}
	instance.chanlen = mincap
	instance.out = make(chan unsafe.Pointer, mincap)
	instance.buffer = make([]unsafe.Pointer, 0)
	return instance
}
func (this *MQ) check() {
	if len(this.buffer) == 0 {
		return
	}
	need := this.chanlen - len(this.out)
	if need == 0 {
		return
	}
	if need >= len(this.buffer) {
		for _, v := range this.buffer {
			this.out <- v
		}
		this.buffer = make([]unsafe.Pointer, 0)
	} else {
		for i, v := range this.buffer {
			if i+1 > need {
				break
			}
			this.out <- v
		}
		this.buffer = this.buffer[need:]
	}
}
func (this *MQ) Put(data unsafe.Pointer) int {
	this.Lock()
	if this.closestatus {
		this.Unlock()
		return len(this.out) + len(this.buffer)
	}
	if len(this.buffer) == 0 {
		if len(this.out) == this.chanlen {
			this.buffer = append(this.buffer, data)
		} else {
			this.out <- data
		}
	} else {
		this.buffer = append(this.buffer, data)
		this.check()
	}
	this.Unlock()
	return len(this.out) + len(this.buffer)
}
func (this *MQ) Get(notice chan int) (unsafe.Pointer, int) {
	if len(notice) > 0 {
		return nil, <-notice
	}
	select {
	case v := <-notice:
		return nil, v
	case v, ok := <-this.out:
		if !ok {
			return nil, 0
		}
		this.Lock()
		if len(this.out) == 0 {
			this.check()
		}
		this.Unlock()
		return v, len(this.out) + len(this.buffer)
	}
}
func (this *MQ) Num() int {
	return len(this.out) + len(this.buffer)
}
func (this *MQ) Close() {
	this.Lock()
	close(this.out)
	this.closestatus = true
	this.Unlock()
}
