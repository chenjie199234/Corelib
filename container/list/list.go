package list

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe
type List struct {
	head *node
	tail *node
}

type node struct {
	value unsafe.Pointer
	next  *node
}

func NewList() *List {
	tempnode := &node{}
	return &List{
		head: tempnode,
		tail: tempnode,
	}
}

func (l *List) Push(data unsafe.Pointer) {
	n := &node{
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
func (l *List) Pop(check func(d unsafe.Pointer) bool) (unsafe.Pointer, bool) {
	for {
		oldhead := l.head
		if oldhead.next == nil {
			return nil, false
		}
		if check != nil && !check(oldhead.next.value) {
			return nil, false
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&l.head)), unsafe.Pointer(oldhead), unsafe.Pointer(oldhead.next)) {
			return oldhead.next.value, true
		}
		runtime.Gosched()
	}
}
