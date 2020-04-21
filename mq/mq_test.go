package mq

import (
	"testing"
	"unsafe"
)

func Test_mq(t *testing.T) {
	mq := NewMQ(128, 1280)
	mq.Reset()
	for i := 0; i < 128*3+1; i++ {
		temp := "123"
		if e := mq.Put(unsafe.Pointer(&temp)); e != nil {
			panic("msg num in mq after put error!")
		}
	}
	if mq.maxlen != 128*2*2 && mq.curlen != 128*3 {
		panic("buffer length in mq after put error!")
	}
	for i := 0; i < 128*3+1; i++ {
		data, left := mq.Get()
		if *(*string)(data) != "123" {
			panic("msg changed!")
		}
		if left != 128*3+1-(i+1) {
			panic("msg num left after get error!")
		}
	}
	mq.Close()
	temp := "123"
	if e := mq.Put(unsafe.Pointer(&temp)); e == nil {
		panic("mq close error!")
	}
	data, left := mq.Get()
	if data != nil || left != -1 {
		panic("mq close error!")
	}
}
