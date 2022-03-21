package list

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe
type List[T any] struct {
	head *node[T]
	tail *node[T]
}

type node[T any] struct {
	value T
	next  *node[T]
}

func NewList[T any]() *List[T] {
	tempnode := &node[T]{}
	return &List[T]{
		head: tempnode,
		tail: tempnode,
	}
}

func (l *List[T]) Push(data T) {
	n := &node[T]{
		value: data,
		next:  nil,
	}
	temptail := l.tail
	for {
		for temptail.next != nil {
			temptail = temptail.next
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&temptail.next)), nil, unsafe.Pointer(n)) {
			break
		}
		runtime.Gosched()
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&l.tail)), unsafe.Pointer(n))
}

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (l *List[T]) Pop(check func(d T) bool) (data T, ok bool) {
	for {
		oldhead := l.head
		if oldhead.next == nil {
			return
		}
		if check != nil && !check(oldhead.next.value) {
			return
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&l.head)), unsafe.Pointer(oldhead), unsafe.Pointer(oldhead.next)) {
			return oldhead.next.value, true
		}
		runtime.Gosched()
	}
}
