package list

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
)

var ErrClosed = errors.New("block list closed")

type BlockList[T any] struct {
	block chan *struct{}
	list  *List[T]
	count int64
}

// work's like golang's chan
func NewBlockList[T any]() *BlockList[T] {
	return &BlockList[T]{
		block: make(chan *struct{}, 1),
		list:  NewList[T](),
		count: 0,
	}
}
func (bl *BlockList[T]) Push(data T) (int64, error) {
	var oldcount int64
	for {
		oldcount = bl.count
		if oldcount < 0 {
			return oldcount + math.MaxInt64, ErrClosed
		}
		if atomic.CompareAndSwapInt64(&bl.count, oldcount, oldcount+1) {
			break
		}
	}
	bl.list.Push(data)
	if oldcount == 0 {
		select {
		case bl.block <- nil:
		default:
		}
	}
	return oldcount + 1, nil
}
func (bl *BlockList[T]) Pop(ctx context.Context) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if ctx.Err() != nil {
			var empty T
			return empty, ctx.Err()
		}
		if data, e := bl.list.Pop(nil); e == nil {
			atomic.AddInt64(&bl.count, -1)
			return data, nil
		}
		if bl.count < 0 {
			var empty T
			return empty, ErrClosed
		}
		select {
		case <-bl.block:
		case <-ctx.Done():
			var empty T
			return empty, ctx.Err()
		}
	}
}
func (bl *BlockList[T]) Count() int64 {
	count := bl.count
	if count < 0 {
		count += math.MaxInt64
	}
	return count
}
func (bl *BlockList[T]) Close() {
	for {
		oldcount := bl.count
		if oldcount < 0 {
			return
		}
		if atomic.CompareAndSwapInt64(&bl.count, oldcount, oldcount-math.MaxInt64) {
			break
		}
	}
	select {
	case bl.block <- nil:
	default:
	}
}
