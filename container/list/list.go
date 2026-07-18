package list

import (
	"errors"
	"runtime"
	"sync/atomic"
)

// thread safe
type List[T any] struct {
	head atomic.Pointer[node[T]]
	tail atomic.Pointer[node[T]]
}

type node[T any] struct {
	value T
	next  atomic.Pointer[node[T]]
}

func NewList[T any]() *List[T] {
	tempnode := &node[T]{}
	l := &List[T]{}
	l.head.Store(tempnode)
	l.tail.Store(tempnode)
	return l
}
func (l *List[T]) Push(data T) {
	n := &node[T]{value: data}
	temptail := l.tail.Load()
	trycas := 0
	for {
		for temptail.next.Load() != nil {
			temptail = temptail.next.Load()
		}
		if temptail.next.CompareAndSwap(nil, n) {
			break
		}
		trycas++
		if trycas >= 3 {
			trycas = 0
			runtime.Gosched()
		}
	}
	l.tail.CompareAndSwap(temptail, n)
}

var ErrPopEmpty = errors.New("pop empty list")
var ErrPopCheckFailed = errors.New("pop list check failed")

// check func is used to check whether the next element can be popped,set nil if don't need it
// if e == ErrPopCheckFailed the data will return but it will not be poped from the list
// Warning!Data returned when check func failed is thread unsafe,maybe another goroutine already popped this data
func (l *List[T]) Pop(check func(d T) bool) (data T, e error) {
	trycas := 0
	for {
		oldhead := l.head.Load()
		oldheadnext := oldhead.next.Load()
		if oldheadnext == nil {
			e = ErrPopEmpty
			return
		}
		if check != nil && !check(oldheadnext.value) {
			data = oldheadnext.value
			e = ErrPopCheckFailed
			return
		}
		if l.head.CompareAndSwap(oldhead, oldheadnext) {
			return oldheadnext.value, nil
		}
		trycas++
		if trycas >= 3 {
			trycas = 0
			runtime.Gosched()
		}
	}
}
