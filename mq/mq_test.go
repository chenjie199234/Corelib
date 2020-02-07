package mq

import (
	"testing"
	"unsafe"
)

func Test_mq(t *testing.T) {
	mq := New(128, 1280)
	for i := 0; i < 128*3+1; i++ {
		temp := "123"
		if left, e := mq.Put(unsafe.Pointer(&temp)); e != nil || left != i+1 {
			panic("msg num in mq after put error!")
		}
	}
	if mq.length != 128*4 {
		panic("buffer length in mq after put error!")
	}
	for i := 0; i < 128*3+1; i++ {
		data, left := mq.Get(nil)
		if *(*string)(data) != "123" {
			panic("msg changed!")
		}
		if left != 128*3+1-(i+1) {
			panic("msg num left after get error!")
		}
	}
	if mq.length != 128 {
		panic("buffer length in mq after getall error!")
	}
	notice := make(chan uint, 1)
	notice <- 1
	data, left := mq.Get(notice)
	if data != nil || left != 1 {
		panic("notice error!")
	}
	mq.Close()
	temp := "123"
	if left, e := mq.Put(unsafe.Pointer(&temp)); e == nil || left != 0 {
		panic("mq close error!")
	}
	data, left = mq.Get(nil)
	if data != nil || left != -1 {
		panic("mq close error!")
	}
}
