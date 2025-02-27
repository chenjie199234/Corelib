package stack

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
)

var ErrClosed = errors.New("block stack closed")

type BlockStack[T any] struct {
	block chan *struct{}
	stack *Stack[T]
	count int64
}

// work's like golang's chan
func NewBlockStack[T any]() *BlockStack[T] {
	return &BlockStack[T]{
		block: make(chan *struct{}, 1),
		stack: NewStack[T](),
		count: 0,
	}
}
func (bs *BlockStack[T]) Push(data T) (int64, error) {
	var oldcount int64
	for {
		oldcount = atomic.LoadInt64(&bs.count)
		if oldcount < 0 {
			return oldcount + math.MaxInt64, ErrClosed
		}
		if atomic.CompareAndSwapInt64(&bs.count, oldcount, oldcount+1) {
			break
		}
	}
	bs.stack.Push(data)
	if oldcount == 0 {
		select {
		case bs.block <- nil:
		default:
		}
	}
	return oldcount + 1, nil
}
func (bs *BlockStack[T]) Pop(ctx context.Context) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if ctx.Err() != nil {
			var empty T
			return empty, ctx.Err()
		}
		if data, e := bs.stack.Pop(nil); e == nil {
			atomic.AddInt64(&bs.count, -1)
			return data, nil
		}
		if atomic.LoadInt64(&bs.count) < 0 {
			var empty T
			return empty, ErrClosed
		}
		select {
		case <-bs.block:
		case <-ctx.Done():
			var empty T
			return empty, ctx.Err()
		}
	}
}
func (bs *BlockStack[T]) Count() int64 {
	count := atomic.LoadInt64(&bs.count)
	if count < 0 {
		count += math.MaxInt64
	}
	return count
}
func (bs *BlockStack[T]) Close() {
	for {
		oldcount := atomic.LoadInt64(&bs.count)
		if oldcount < 0 {
			return
		}
		if atomic.CompareAndSwapInt64(&bs.count, oldcount, oldcount-math.MaxInt64) {
			break
		}
	}
	select {
	case bs.block <- nil:
	default:
	}
}
