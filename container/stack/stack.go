package stack

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe
type Stack struct {
	top *node
}

type node struct {
	value unsafe.Pointer
	pre   *node
}

func NewStack() *Stack {
	return &Stack{
		top: &node{},
	}
}
func (s *Stack) Push(data unsafe.Pointer) {
	n := &node{
		value: data,
		pre:   s.top,
	}
	for !atomic.CompareAndSwapPointer((*unsafe.Pointer)((unsafe.Pointer)(&s.top)), unsafe.Pointer(n.pre), unsafe.Pointer(n)) {
		n.pre = s.top
	}
}

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (s *Stack) Pop(check func(d unsafe.Pointer) bool) (unsafe.Pointer, bool) {
	for {
		oldtop := s.top
		if oldtop.pre == nil {
			return nil, false
		}
		if check != nil && !check(oldtop.value) {
			return nil, false
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top)), unsafe.Pointer(oldtop), unsafe.Pointer(oldtop.pre)) {
			return oldtop.value, true
		}
		runtime.Gosched()
	}
}
