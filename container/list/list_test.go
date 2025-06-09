package list

import (
	"sync"
	"testing"
	"time"
)

func Test_List(T *testing.T) {
	l := NewList[int]()
	start := make(chan int)
	go func() {
		<-start
		for i := range 10000 {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 10000; i < 20000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 20000; i < 30000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 30000; i < 40000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 40000; i < 50000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 50000; i < 60000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 60000; i < 70000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 70000; i < 80000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 80000; i < 90000; i++ {
			l.Push(i)
		}
	}()
	go func() {
		<-start
		for i := 90000; i < 100000; i++ {
			l.Push(i)
		}
	}()
	wg := sync.WaitGroup{}
	lker := &sync.RWMutex{}
	exist := make(map[int]*struct{})
	f := func(checkvalue *int) {
		for {
			d, e := l.Pop(func(d int) bool {
				return d < *checkvalue
			})
			if e != nil {
				if e == ErrPopEmpty {
					lker.RLock()
					if len(exist) == 100000 {
						lker.RUnlock()
						return
					}
					lker.RUnlock()
				} else if e == ErrPopCheckFailed {
					*checkvalue = 100000
				}
			} else {
				lker.Lock()
				if _, ok := exist[d]; ok {
					panic("duplicate data")
				} else {
					exist[d] = nil
				}
				lker.Unlock()
			}
		}
	}
	wg.Add(1)
	go func() {
		checkvalue := 10000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 20000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 30000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 40000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 50000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 60000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 70000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 80000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		checkvalue := 90000
		<-start
		f(&checkvalue)
		wg.Done()
	}()
	time.Sleep(time.Millisecond)
	close(start)
	wg.Wait()
	T.Log(len(exist))
}
