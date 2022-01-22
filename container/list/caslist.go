package list

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe,without lock,but memory not friendly,gc will increase
type CasList struct {
	head *node
	tail *node
}

func NewCasList() *CasList {
	tempnode := &node{}
	return &CasList{
		head: tempnode,
		tail: tempnode,
	}
}

//push back
func (l *CasList) Push(data unsafe.Pointer) {
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

//pop front
func (l *CasList) Pop() unsafe.Pointer {
	for {
		oldhead := l.head
		if oldhead.next == nil {
			return nil
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&l.head)), unsafe.Pointer(oldhead), unsafe.Pointer(oldhead.next)) {
			return oldhead.next.value
		}
		runtime.Gosched()
	}
}
func (l *CasList) GetHead() unsafe.Pointer {
	return l.head.value
}
func (l *CasList) GetTail() unsafe.Pointer {
	return l.tail.value
}
