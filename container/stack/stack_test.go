package stack

import (
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func Benchmark_Cas(b *testing.B) {
	b.StopTimer()
	l := NewStack()
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
