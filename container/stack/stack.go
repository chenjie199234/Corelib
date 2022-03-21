package stack

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe
type Stack[T any] struct {
	top *node[T]
}

type node[T any] struct {
	value T
	pre   *node[T]
}

func NewStack[T any]() *Stack[T] {
	return &Stack[T]{
		top: &node[T]{},
	}
}
func (s *Stack[T]) Push(data T) {
	n := &node[T]{
		value: data,
		pre:   s.top,
	}
	for !atomic.CompareAndSwapPointer((*unsafe.Pointer)((unsafe.Pointer)(&s.top)), unsafe.Pointer(n.pre), unsafe.Pointer(n)) {
		n.pre = s.top
	}
}

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (s *Stack[T]) Pop(check func(d T) bool) (data T, ok bool) {
	for {
		oldtop := s.top
		if oldtop.pre == nil {
			return
		}
		if check != nil && !check(oldtop.value) {
			return
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top)), unsafe.Pointer(oldtop), unsafe.Pointer(oldtop.pre)) {
			return oldtop.value, true
		}
		runtime.Gosched()
	}
}
