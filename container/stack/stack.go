package stack

import (
	"errors"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// thread safe
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
		pre:   (*node[T])(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top)))),
	}
	for !atomic.CompareAndSwapPointer((*unsafe.Pointer)((unsafe.Pointer)(&s.top)), unsafe.Pointer(n.pre), unsafe.Pointer(n)) {
		n.pre = (*node[T])(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top))))
	}
}

var ErrPopEmpty = errors.New("pop empty stack")
var ErrPopCheckFailed = errors.New("pop stack check failed")

// check func is used to check whether the next element can be popped,set nil if don't need it
// if e == ErrPopCheckFailed the data will return but it will not be poped from the stack
func (s *Stack[T]) Pop(check func(d T) bool) (data T, e error) {
	for {
		oldtop := (*node[T])(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top))))
		oldtoppre := (*node[T])(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&oldtop.pre))))
		if oldtoppre == nil {
			e = ErrPopEmpty
			return
		}
		if check != nil && !check(oldtop.value) {
			data = oldtop.value
			e = ErrPopCheckFailed
			return
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.top)), unsafe.Pointer(oldtop), unsafe.Pointer(oldtop.pre)) {
			return oldtop.value, nil
		}
		runtime.Gosched()
	}
}
