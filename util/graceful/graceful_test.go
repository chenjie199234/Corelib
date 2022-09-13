package graceful

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_Graceful(t *testing.T) {
	g := NewGraceful()
	g.AddOne()
	start := time.Now().Unix()
	go func() {
		defer g.DoneOne()
		time.Sleep(time.Second * 2)
	}()
	wg := &sync.WaitGroup{}
	wg.Add(2)
	closenum := int32(0)
	go func() {
		defer wg.Done()
		g.Close()
		atomic.AddInt32(&closenum, 1)
	}()
	go func() {
		defer wg.Done()
		g.Close()
		atomic.AddInt32(&closenum, 1)
	}()
	wg.Wait()
	end := time.Now().Unix()
	if end-start < 2 {
		t.Fatal("didn't wait")
	}
	if closenum != 2 {
		t.Fatal("not all close return")
	}
}
