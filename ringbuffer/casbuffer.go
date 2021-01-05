package ringbuffer

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe,but the capacity is fixed
type CasRingBuffer struct {
	head     uint32
	tail     uint32
	buf      []*node
	capacity uint32
}

type node struct {
	getpos uint32
	putpos uint32
	value  unsafe.Pointer
}

func NewCasRingBuffer(num uint32) *CasRingBuffer {
	buf := &CasRingBuffer{
		head:     0,
		tail:     0,
		buf:      make([]*node, num),
		capacity: num,
	}
	for i := range buf.buf {
		buf.buf[i] = &node{
			getpos: uint32(i),
			putpos: uint32(i),
			value:  nil,
		}
	}
	return buf
}

func (a *CasRingBuffer) Push(data unsafe.Pointer) error {
	var putpos uint32
	for {
		putpos = a.tail
		if a.head+a.capacity-putpos < 1 {
			return ERRFULL
		}
		if atomic.CompareAndSwapUint32(&a.tail, putpos, putpos+1) {
			break
		}
	}
	element := a.buf[putpos%a.capacity]
	for {
		if element.putpos == putpos && element.putpos == element.getpos {
			element.value = data
			atomic.AddUint32(&element.putpos, a.capacity)
			return nil
		} else {
			runtime.Gosched()
		}
	}
}

func (a *CasRingBuffer) Pushs(datas []unsafe.Pointer) error {
	var putpos uint32
	for {
		putpos = a.tail
		if int(a.head+a.capacity-putpos) < len(datas) {
			return ERRFULL
		}
		if atomic.CompareAndSwapUint32(&a.tail, putpos, putpos+uint32(len(datas))) {
			break
		}
	}
	for i, data := range datas {
		element := a.buf[(putpos+uint32(i))%a.capacity]
		for {
			if element.putpos == putpos && element.putpos == element.getpos {
				element.value = data
				atomic.AddUint32(&element.putpos, a.capacity)
				break
			} else {
				runtime.Gosched()
			}
		}
	}
	return nil
}

//return nil means empty
func (a *CasRingBuffer) Pop() unsafe.Pointer {
	var getpos uint32
	for {
		getpos = a.head
		if getpos+1 > a.tail {
			return nil
		}
		if atomic.CompareAndSwapUint32(&a.head, getpos, getpos+1) {
			break
		}
	}
	element := a.buf[getpos%a.capacity]
	for {
		if element.getpos == getpos && element.putpos-a.capacity == getpos {
			val := element.value
			element.value = nil
			atomic.AddUint32(&element.getpos, a.capacity)
			return val
		} else {
			runtime.Gosched()
		}
	}
}

//return nil means required too much
func (a *CasRingBuffer) Pops(num uint32) []unsafe.Pointer {
	var getpos uint32
	for {
		getpos = a.head
		if getpos+num > a.tail {
			return nil
		}
		if atomic.CompareAndSwapUint32(&a.head, getpos, getpos+num) {
			break
		}
	}
	result := make([]unsafe.Pointer, num)
	for i := uint32(0); i < num; i++ {
		element := a.buf[(getpos+uint32(i))%a.capacity]
		for {
			if element.getpos == getpos && element.putpos-a.capacity == getpos {
				result[i] = element.value
				element.value = nil
				atomic.AddUint32(&element.getpos, a.capacity)
				break
			} else {
				runtime.Gosched()
			}
		}
	}
	return result
}
