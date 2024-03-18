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
				l.Close()
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
	if _, e := l.Push(11); e != ErrClosed {
		t.Fatal("should return error closed")
		return
	}
}
