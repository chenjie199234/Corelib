package ringbuffer

import (
	"sync/atomic"
	"unsafe"
)

//thread safe,but the capacity is fixed
type CasRingBuffer struct {
	head     uint32
	tail     uint32
	buf      []unsafe.Pointer
	capacity uint32
}

func NewCasRingBuffer(num uint32) *CasRingBuffer {
	return &CasRingBuffer{
		head:     0,
		tail:     0,
		buf:      make([]unsafe.Pointer, num),
		capacity: num,
	}
}

func (a *CasRingBuffer) Push(data unsafe.Pointer) error {
	for {
		oldtail := a.tail
		if a.head+a.capacity-oldtail < 1 {
			//full
			return ERRFULL
		}
		newtail := oldtail + 1
		if atomic.CompareAndSwapUint32(&a.tail, oldtail, newtail) {
			a.buf[oldtail%a.capacity] = data
			return nil
		}
	}
}

func (a *CasRingBuffer) Pushs(datas []unsafe.Pointer) error {
	if uint32(len(datas)) > a.capacity {
		return ERRFULL
	}
	for {
		oldtail := a.tail
		if a.head+a.capacity-oldtail < uint32(len(datas)) {
			return ERRFULL
		}
		newtail := oldtail + uint32(len(datas))
		if atomic.CompareAndSwapUint32(&a.tail, oldtail, newtail) {
			for i := uint32(0); i < uint32(len(datas)); i++ {
				a.buf[(oldtail+i)%a.capacity] = datas[i]
			}
			return nil
		}
	}
}

//return nil means empty
func (a *CasRingBuffer) Pop() unsafe.Pointer {
	for {
		oldhead := a.head
		if oldhead+1 > a.tail {
			return nil
		}
		newhead := oldhead + 1
		if atomic.CompareAndSwapUint32(&a.head, oldhead, newhead) {
			return a.buf[oldhead%a.capacity]
		}
	}
}

//return nil means don't have enough elements
func (a *CasRingBuffer) Pops(num uint32) []unsafe.Pointer {
	if num > a.capacity {
		return nil
	}
	for {
		oldhead := a.head
		if oldhead+num > a.tail {
			return nil
		}
		newhead := oldhead + num
		if atomic.CompareAndSwapUint32(&a.head, oldhead, newhead) {
			result := make([]unsafe.Pointer, num)
			for i := uint32(0); i < num; i++ {
				result[i] = a.buf[(oldhead+i)%a.capacity]
			}
			return result
		}
	}
}
