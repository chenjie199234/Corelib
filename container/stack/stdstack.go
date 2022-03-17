package stack

import (
	"sync"
	"unsafe"
)

//thread not safe
type StdStack struct {
	top  *node
	pool *sync.Pool
}

func (s *StdStack) getnode(data unsafe.Pointer) *node {
	n, ok := s.pool.Get().(*node)
	if !ok {
		return &node{value: data, pre: nil}
	}
	n.value = data
	return n
}
func (s *StdStack) putnode(n *node) {
	n.value = nil
	n.pre = nil
	s.pool.Put(n)
}

func NewStdStack() *StdStack {
	return &StdStack{
		pool: &sync.Pool{},
		top:  &node{},
	}
}
func (s *StdStack) Push(data unsafe.Pointer) {
	n := s.getnode(data)
	n.pre = s.top
	s.top = n
}

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (s *StdStack) Pop(check func(d unsafe.Pointer) bool) (unsafe.Pointer, bool) {
	oldtop := s.top
	if oldtop.pre == nil {
		return nil, false
	}
	if check != nil && !check(oldtop.value) {
		return nil, false
	}
	s.top = oldtop.pre
	result := oldtop.value
	s.putnode(oldtop)
	return result, true
}
