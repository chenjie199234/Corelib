package ringbuffer

import (
	"testing"
	"unsafe"
)

func Test_RingBuffer(t *testing.T) {
	buf := NewRingBuffer(10, 100)
	a := "a"
	b := "b"
	buf.Put([]unsafe.Pointer{unsafe.Pointer(&a), unsafe.Pointer(&b)})
	if buf.Num() != 2 {
		panic("num error")
	}
	if buf.Rest() != 98 {
		panic("rest error")
	}
	if *(*string)(buf.Peek(1, 1)[0]) != "b" {
		panic("peek error")
	}
	if *(*string)(buf.Get(1)[0]) != "a" {
		panic("get error")
	}
	if buf.Num() != 1 {
		panic("num error")
	}
	if buf.Rest() != 99 {
		panic("rest error")
	}
	for i := 0; i < 10; i++ {
		buf.Put([]unsafe.Pointer{unsafe.Pointer(&b)})
	}
	if buf.Num() != 11 {
		panic("num error")
	}
	if buf.Rest() != 89 {
		panic("rest error")
	}
	for i := 0; i < 89; i++ {
		buf.Put([]unsafe.Pointer{unsafe.Pointer(&b)})
	}
	if buf.Num() != 100 {
		panic("num error")
	}
	if buf.Rest() != 0 {
		panic("rest error")
	}
	e := buf.Put([]unsafe.Pointer{unsafe.Pointer(&b)})
	if e != ERRFULL {
		panic("full error")
	}
}
