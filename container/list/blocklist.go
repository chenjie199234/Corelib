package list

import (
	"math"
	"sync/atomic"
)

type BlockList[T any] struct {
	block chan *struct{}
	list  *List[T]
	count uint64
}

// work's like golang's chan
func NewBlockList[T any]() *BlockList[T] {
	return &BlockList[T]{
		block: make(chan *struct{}, 1),
		list:  NewList[T](),
		count: 0,
	}
}
func (bl *BlockList[T]) Push(data T) uint64 {
	bl.list.Push(data)
	newcount := atomic.AddUint64(&bl.count, 1)
	if newcount == 1 {
		select {
		case bl.block <- nil:
		default:
		}
	}
	return newcount
}
func (bl *BlockList[T]) Pop() (T, bool) {
	for {
		if data, e := bl.list.Pop(nil); e == nil {
			atomic.AddUint64(&bl.count, math.MaxUint64)
			return data, true
		}
		if _, ok := <-bl.block; !ok {
			var empty T
			return empty, false
		}
	}
}
func (bl *BlockList[T]) Stop() {
	close(bl.block)
}
