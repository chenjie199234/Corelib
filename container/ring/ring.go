package ring

import (
	"errors"
	"runtime"
	"sync/atomic"
)

// thread safe
type Ring[T any] struct {
	length, popTry, popConfirm, pushTry, pushConfirm uint64
	data                                             []T
}

func NewRing[T any](length uint64) *Ring[T] {
	return &Ring[T]{
		length: length,
		data:   make([]T, length),
	}
}

// return false - only when the buf is full
func (b *Ring[T]) Push(d T) bool {
	for {
		oldPushTry := atomic.LoadUint64(&b.pushTry)
		if oldPushTry-atomic.LoadUint64(&b.popConfirm) == atomic.LoadUint64(&b.length) {
			//full
			return false
		}
		if !atomic.CompareAndSwapUint64(&b.pushTry, oldPushTry, oldPushTry+1) {
			continue
		}
		b.data[oldPushTry%atomic.LoadUint64(&b.length)] = d
		for !atomic.CompareAndSwapUint64(&b.pushConfirm, oldPushTry, oldPushTry+1) {
			runtime.Gosched()
		}
		return true
	}
}

var ErrPopEmpty = errors.New("pop empty ring")
var ErrPopCheckFailed = errors.New("pop ring check failed")

// check func is used to check whether the next element can be popped,set nil if don't need it
// if e == ErrPopCheckFailed the data will return but it will not be poped from the ring
func (b *Ring[T]) Pop(check func(d T) bool) (data T, e error) {
	for {
		oldPopTry := atomic.LoadUint64(&b.popTry)
		if oldPopTry == atomic.LoadUint64(&b.pushConfirm) {
			e = ErrPopEmpty
			return
		}
		d := b.data[oldPopTry%atomic.LoadUint64(&b.length)]
		if check != nil && !check(d) {
			data = d
			e = ErrPopCheckFailed
			return
		}
		if !atomic.CompareAndSwapUint64(&b.popTry, oldPopTry, oldPopTry+1) {
			continue
		}
		for !atomic.CompareAndSwapUint64(&b.popConfirm, oldPopTry, oldPopTry+1) {
			runtime.Gosched()
		}
		return d, nil
	}
}
