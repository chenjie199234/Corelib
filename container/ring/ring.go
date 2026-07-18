package ring

import (
	"errors"
	"runtime"
	"sync/atomic"
)

// thread safe
type Ring[T any] struct {
	pushseq, popseq uint32
	nodes           []*node[T]
}

type node[T any] struct {
	status atomic.Bool
	value  T
}

// if length == 0,nil will be returned
func NewRing[T any](length uint32) *Ring[T] {
	if length == 0 {
		return nil
	}
	r := &Ring[T]{nodes: make([]*node[T], length)}
	for i := range length {
		r.nodes[i] = &node[T]{}
	}
	return r
}

// return false - only when the buf is full
func (r *Ring[T]) Push(d T) bool {
	try := 0
	for {
		oldseq := atomic.LoadUint32(&r.pushseq)
		if oldseq-atomic.LoadUint32(&r.popseq) >= uint32(len(r.nodes)) {
			return false
		}
		index := oldseq % uint32(len(r.nodes))
		if r.nodes[index].status.Load() {
			//pop not finished
			try++
			if try >= 3 {
				try = 0
				runtime.Gosched()
			}
			continue
		}
		if !atomic.CompareAndSwapUint32(&r.pushseq, oldseq, oldseq+1) {
			try++
			if try >= 3 {
				try = 0
				runtime.Gosched()
			}
			continue
		}
		r.nodes[index].value = d
		r.nodes[index].status.Store(true)
		return true
	}
}

var ErrPopEmpty = errors.New("pop empty ring")
var ErrPopCheckFailed = errors.New("pop ring check failed")

// check func is used to check whether the next element can be popped,set nil if don't need it
// if e == ErrPopCheckFailed the data will return but it will not be poped from the ring
// Warning!Data returned when check func failed is thread unsafe,maybe another goroutine already popped this data
func (r *Ring[T]) Pop(check func(d T) bool) (data T, e error) {
	try := 0
	for {
		oldseq := atomic.LoadUint32(&r.popseq)
		if atomic.LoadUint32(&r.pushseq)-oldseq == 0 {
			e = ErrPopEmpty
			return
		}
		index := oldseq % uint32(len(r.nodes))
		if !r.nodes[index].status.Load() {
			//push not finished
			//or
			//another goroutine already popped this slot
			try++
			if try >= 3 {
				try = 0
				runtime.Gosched()
			}
			continue
		}
		data = r.nodes[index].value
		if check != nil && !check(data) {
			if atomic.LoadUint32(&r.popseq) != oldseq {
				//this data already popped
				try++
				if try >= 3 {
					try = 0
					runtime.Gosched()
				}
				continue
			}
			e = ErrPopCheckFailed
			return
		}
		if !atomic.CompareAndSwapUint32(&r.popseq, oldseq, oldseq+1) {
			try++
			if try >= 3 {
				try = 0
				runtime.Gosched()
			}
			continue
		}
		r.nodes[index].status.Store(false)
		return
	}
}

/*

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

*/
