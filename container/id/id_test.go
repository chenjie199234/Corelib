package id

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var data map[uint64]struct{}

func Test_Id(t *testing.T) {
	lker := new(sync.Mutex)
	data = make(map[uint64]struct{}, 1000)
	New(1)
	for i := 0; i < 1000; i++ {
		go func(index int) {
			for {
				time.Sleep(time.Millisecond)
				id, e := GetID()
				if e != nil {
					panic(e)
				}
				lker.Lock()
				if _, ok := data[id]; ok {
					panic(fmt.Sprintf("dup:%d len:%d", id, len(data)))
				}
				data[id] = struct{}{}
				lker.Unlock()
			}
		}(i)
	}
	go func() {
		for {
			time.Sleep(time.Second)
			lker.Lock()
			fmt.Println("created id total num:", len(data))
			lker.Unlock()
		}
	}()
	select {}
}
