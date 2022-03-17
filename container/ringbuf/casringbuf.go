package ringbuf

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

//thread safe
type CasRingBuf struct {
	length, popTry, popConfirm, pushTry, pushConfirm uint64
	data                                             []unsafe.Pointer
}

func NewCasRingBuf(length uint64) *CasRingBuf {
	return &CasRingBuf{
		length: length,
		data:   make([]unsafe.Pointer, length),
	}
}

//return false - only when the buf is full
func (b *CasRingBuf) Push(d unsafe.Pointer) bool {
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

//check func is used to check whether the next element can be popped,set nil if don't need it
//return false - when the buf is empty,or the check failed
func (b *CasRingBuf) Pop(check func(d unsafe.Pointer) bool) (unsafe.Pointer, bool) {
	for {
		oldPopTry := atomic.LoadUint64(&b.popTry)
		if oldPopTry == atomic.LoadUint64(&b.pushConfirm) {
			return nil, false
		}
		d := b.data[oldPopTry%atomic.LoadUint64(&b.length)]
		if check != nil && !check(d) {
			return nil, false
		}
		if !atomic.CompareAndSwapUint64(&b.popTry, oldPopTry, oldPopTry+1) {
			continue
		}
		for !atomic.CompareAndSwapUint64(&b.popConfirm, oldPopTry, oldPopTry+1) {
			runtime.Gosched()
		}
		return d, true
	}
}
