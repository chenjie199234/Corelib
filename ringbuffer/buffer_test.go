package ringbuffer

import (
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

var d uint32

func Benchmark_Cas(b *testing.B) {
	b.StopTimer()
	buf := NewCasRingBuffer(40960)
	wg := &sync.WaitGroup{}
	var count uint32
	b.ResetTimer()
	b.StartTimer()
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100000; j++ {
				for buf.Push(unsafe.Pointer(&j)) != nil {
				}
			}
		}()
		go func() {
			defer wg.Done()
			for {
				r := buf.Pop()
				if r != nil {
					atomic.AddUint32(&count, 1)
				}
				if d := atomic.LoadUint32(&count); d == 1000000 {
					return
				}
			}
		}()
	}
	wg.Wait()
}
func Benchmark_Std(b *testing.B) {
	b.StopTimer()
	buf := NewStdRingBuffer(1024, 40960)
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
				for {
					lker.Lock()
					e := buf.Push(unsafe.Pointer(&j))
					lker.Unlock()
					if e == nil {
						break
					}
				}
			}
		}()
		go func() {
			defer wg.Done()
			for {
				lker.Lock()
				r := buf.Pop()
				lker.Unlock()
				if r != nil {
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
func Benchmark_Chan(b *testing.B) {
	b.StopTimer()
	ch := make(chan unsafe.Pointer, 40960)
	wg := &sync.WaitGroup{}
	var count uint32
	b.ResetTimer()
	b.StartTimer()
	wg.Add(20)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100000; j++ {
				ch <- unsafe.Pointer(&j)
			}
		}()
		go func() {
			defer wg.Done()
			for {
				<-ch
				if atomic.AddUint32(&count, 1) == 1000000 {
					close(ch)
				}
				if count >= 1000000 {
					return
				}
			}
		}()
	}
	wg.Wait()
}
