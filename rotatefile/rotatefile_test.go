package rotatefile

import (
	"bytes"
	"testing"
	"time"
)

func Test_RotateFile(t *testing.T) {
	r, e := NewRotateFile("./log", "test")
	if e != nil {
		panic(e)
	}
	for i := 0; i < 100; i++ {
		r.Write(append(bytes.Repeat([]byte("a"), 100), '\n'))
	}
	time.Sleep(time.Second * 2)
	for i := 0; i < 100; i++ {
		r.Write(append(bytes.Repeat([]byte("b"), 100), '\n'))
	}
	if e := r.RotateNow(); e != nil {
		panic(e)
	}
	for i := 0; i < 100; i++ {
		r.Write(bytes.Repeat([]byte("a"), 100))
		r.Write([]byte("\n"))
	}
	time.Sleep(time.Second * 2)
	for i := 0; i < 100; i++ {
		r.Write(append(bytes.Repeat([]byte("b"), 100), '\n'))
	}
	time.Sleep(time.Second)
	r.CleanNow(time.Now().UnixNano())
	r.Close()
}
