package stack

import (
	"sync"
	"unsafe"
)

//thread not safe,memory friendly,gc will reduce,but has lock
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
	temp := &node{}
	return &StdStack{
		pool: &sync.Pool{},
		top:  temp,
	}
}
func (s *StdStack) Push(data unsafe.Pointer) {
	n := s.getnode(data)
	n.pre = s.top
	s.top = n
}
func (s *StdStack) Pop() unsafe.Pointer {
	var temp *node
	result := s.top.value
	if s.top.pre != nil {
		temp = s.top
		s.top = s.top.pre
	}
	if temp != nil {
		s.putnode(temp)
	}
	return result
}
