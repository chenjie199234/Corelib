package stack

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe,without lock,but memory not friendly,gc will increase
type CasStack struct {
	top *node
}

type node struct {
	value unsafe.Pointer
	pre   *node
}

func NewCasStack() *CasStack {
	return &CasStack{
		top: &node{},
	}
}
func (s *CasStack) Push(data unsafe.Pointer) {
	n := &node{
		value: data,
		pre:   s.top,
	}
	for !atomic.CompareAndSwapPointer((*unsafe.Pointer)((unsafe.Pointer)(&s.top)), unsafe.Pointer(n.pre), unsafe.Pointer(n)) {
		n.pre = s.top
	}
}
func (s *CasStack) Pop() unsafe.Pointer {
	for {
		oldtop := s.top
		if oldtop.pre == nil {
			return nil
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top)), unsafe.Pointer(oldtop), unsafe.Pointer(oldtop.pre)) {
			return oldtop.value
		}
		runtime.Gosched()
	}
}
