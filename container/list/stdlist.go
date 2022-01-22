package list

import (
	"sync"
	"unsafe"
)

//thread not safe,memory friendly,gc will reduce
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
func (l *StdList) Pop() unsafe.Pointer {
	if l.head.next == nil {
		return nil
	}
	temp := l.head
	l.head = l.head.next
	l.putnode(temp)
	return l.head.value
}
func (l *StdList) GetHead() unsafe.Pointer {
	return l.head.value
}
func (l *StdList) GetTail() unsafe.Pointer {
	return l.tail.value
}
