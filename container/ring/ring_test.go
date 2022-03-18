package ring

import (
	"testing"
	"unsafe"
)

func Test_RingBuf(t *testing.T) {
	buf := NewRing(1)
	a := 1
	if !buf.Push(unsafe.Pointer(&a)) {
		t.Fatal("push first failed,should success")
	}
	if buf.Push(unsafe.Pointer(&a)) {
		t.Fatal("push seccend success,should fail")
	}
	_, ok := buf.Pop(func(d unsafe.Pointer) bool {
		return false
	})
	if ok {
		t.Fatal("pop success,should fail")
	}
	_, ok = buf.Pop(nil)
	if !ok {
		t.Fatal("pop fail,should success")
	}
	if !buf.Push(unsafe.Pointer(&a)) {
		t.Fatal("push fail,should success")
	}
	_, ok = buf.Pop(func(unsafe.Pointer) bool {
		return true
	})
	if !ok {
		t.Fatal("pop fail,should success")
	}
}
