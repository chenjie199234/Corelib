package rotatefile

import (
	"strings"
	"sync"
	"testing"

	"github.com/chenjie199234/Corelib/bufpool"
)

func Test_RotateFile(t *testing.T) {
	f, e := NewRotateFile("./", "abc", 1, 0)
	if e != nil {
		panic(e)
	}
	wg := &sync.WaitGroup{}
	wg.Add(500)
	cond := sync.NewCond(&sync.Mutex{})
	for c := 0; c < 500; c++ {
		go func() {
			defer wg.Done()
			cond.L.Lock()
			wg.Done()
			cond.Wait()
			for i := 0; i < 1000; i++ {
				buf := bufpool.GetBuffer()
				buf.Append(strings.Repeat("a", 15) + "\n")
				if _, e := f.WriteBuf(buf); e != nil {
					panic(e)
				}
			}
			cond.L.Unlock()
		}()
	}
	wg.Wait()
	t.Log("prepared")
	wg.Add(500)
	cond.Broadcast()
	wg.Wait()
	f.Close(true)
	buf := bufpool.GetBuffer()
	buf.Append("1")
	if _, e := f.WriteBuf(buf); e == nil {
		panic("file already closed,write should return error,but no error return")
	}
}
