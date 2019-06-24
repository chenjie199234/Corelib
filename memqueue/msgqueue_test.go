package memqueue

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func Test_mq(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().Unix())
	mq := New(128)
	wg := new(sync.WaitGroup)
	for count := 0; count < 10; count++ {
		wg.Add(1)
		go putf(mq, wg)
	}
	wg.Wait()
	runtime.GC()
	udata, n := mq.Get(nil)
	if n != 9999 {
		panic("count error")
	}
	if *(*string)(udata) != "123456789" {
		panic("content error")
	}
	runtime.GC()
	udata, n = mq.Get(nil)
	if n != 9998 {
		panic("count error")
	}
	if *(*string)(udata) != "123456789" {
		panic("content error")
	}
}
func putf(mq *MQ, wg *sync.WaitGroup) {
	defer wg.Done()
	for count := 0; count < 1000; count++ {
		str := "123456789"
		mq.Put(unsafe.Pointer(&str))
		time.Sleep(time.Duration(rand.Int()%20) * time.Millisecond)
	}
}
