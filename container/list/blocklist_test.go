package list

import (
	"testing"
	"time"
)

func Test_BlockList(t *testing.T) {
	l := NewBlockList[int]()
	go func() {
		count := 0
		for {
			time.Sleep(time.Second)
			l.Push(count)
			count++
			if count == 10 {
				l.Stop()
				return
			}
		}
	}()
	for {
		data, ok := l.Pop()
		if !ok {
			break
		}
		t.Log(data)
	}
}
