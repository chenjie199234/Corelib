package stack

import (
	"errors"
	"runtime"
	"sync/atomic"
)

// thread safe
type Stack[T any] struct {
	top atomic.Pointer[node[T]]
}

type node[T any] struct {
	value T
	pre   *node[T]
}

func NewStack[T any]() *Stack[T] {
	s := &Stack[T]{}
	s.top.Store(&node[T]{})
	return s
}
func (s *Stack[T]) Push(data T) {
	n := &node[T]{value: data, pre: s.top.Load()}
	trycas := 0
	for !s.top.CompareAndSwap(n.pre, n) {
		n.pre = s.top.Load()
		trycas++
		if trycas >= 3 {
			trycas = 0
			runtime.Gosched()
		}
	}
}

var ErrPopEmpty = errors.New("pop empty stack")
var ErrPopCheckFailed = errors.New("pop stack check failed")

// check func is used to check whether the next element can be popped,set nil if don't need it
// if e == ErrPopCheckFailed the data will return but it will not be poped from the stack
// Warning!Data returned when check func failed is thread unsafe,maybe another goroutine already popped this data
func (s *Stack[T]) Pop(check func(d T) bool) (data T, e error) {
	trycas := 0
	for {
		oldtop := s.top.Load()
		if oldtop.pre == nil {
			e = ErrPopEmpty
			return
		}
		if check != nil && !check(oldtop.value) {
			data = oldtop.value
			e = ErrPopCheckFailed
			return
		}
		if s.top.CompareAndSwap(oldtop, oldtop.pre) {
			return oldtop.value, nil
		}
		trycas++
		if trycas >= 3 {
			trycas = 0
			runtime.Gosched()
		}
	}
}
