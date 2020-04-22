package buffer

import (
	"bytes"
	"testing"
)

func TestBuffer(t *testing.T) {
	b := NewBuf(10, 100)
	//len 52
	bb := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b.Put(bb)
	//after put 10*2*2*2 = 80
	if !bytes.Equal(b.data[:len(bb)], bb) {
		panic("first put error")
	}
	if !bytes.Equal(b.Peek(1, 2), []byte("bc")) {
		panic("peek normal error")
	}
	if b.Peek(70, 2) != nil {
		panic("peek overflow error")
	}
	if b.Num() != len(bb) {
		panic("num error")
	}
	if b.head != 0 {
		panic("head error")
	}
	if b.tail != len(bb) {
		panic("tail error")
	}
	if !bytes.Equal(b.Get(2), []byte("ab")) {
		panic("first get error")
	}
	bbb := bytes.Repeat([]byte("1"), 30)
	b.Put(bbb)
	if b.maxlen != 80 || b.curlen != 80 || b.head != 2 || b.tail != 2 {
		panic("second put error")
	}
	b.Put([]byte("22"))
	if b.maxlen != 100 || b.curlen != 82 || b.head != 0 || b.tail != 82 {
		panic("third put error")
	}
	b.Get(82)
	if b.maxlen != 100 || b.curlen != 0 || b.head != 82 || b.tail != 82 {
		panic("second get error")
	}
}
