package list

import (
	"sync"
	"unsafe"
)

//thread not safe
type StdList struct {
	head *node
	tail *node
	pool *sync.Pool
}

func NewStdList() *StdList {
	temp := &node{}
	return &StdList{
		head: temp,
		tail: temp,
		pool: &sync.Pool{},
	}
}

func (l *StdList) getnode(data unsafe.Pointer) *node {
	n, ok := l.pool.Get().(*node)
	if !ok {
		return &node{value: data, next: nil}
	}
	n.value = data
	return n
}
func (l *StdList) putnode(n *node) {
	n.next = nil
	n.value = nil
	l.pool.Put(n)
}

func (l *StdList) Push(data unsafe.Pointer) {
	node := l.getnode(data)
	l.tail.next = node
	l.tail = node
}

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (l *StdList) Pop(check func(d unsafe.Pointer) bool) (unsafe.Pointer, bool) {
	oldhead := l.head
	if oldhead.next == nil {
		return nil, false
	}
	if check != nil && !check(oldhead.value) {
		return nil, false
	}
	l.head = oldhead.next
	result := oldhead.value
	l.putnode(oldhead)
	return result, true
}
