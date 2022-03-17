package stack

import (
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func Benchmark_Cas(b *testing.B) {
	b.StopTimer()
	l := NewCasStack()
	wg := &sync.WaitGroup{}
	var count uint32
	b.ResetTimer()
	b.StartTimer()
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100000; j++ {
				l.Push(unsafe.Pointer(&j))
			}
		}()
		go func() {
			defer wg.Done()
			for {
				if _, ok := l.Pop(nil); ok {
					atomic.AddUint32(&count, 1)
				}
				if atomic.LoadUint32(&count) == 1000000 {
					return
				}
			}
		}()
	}
	wg.Wait()
}

func Benchmark_Std(b *testing.B) {
	b.StopTimer()
	l := NewStdStack()
	lker := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	var count uint32
	b.ResetTimer()
	b.StartTimer()
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100000; j++ {
				lker.Lock()
				l.Push(unsafe.Pointer(&j))
				lker.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			for {
				lker.Lock()
				_, ok := l.Pop(nil)
				lker.Unlock()
				if ok {
					atomic.AddUint32(&count, 1)
				}
				if atomic.LoadUint32(&count) == 1000000 {
					return
				}
			}
		}()
	}
	wg.Wait()
}
