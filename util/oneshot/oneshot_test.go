package oneshot

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func Test_Once(t *testing.T) {
	wg := new(sync.WaitGroup)
	wg.Add(10)
	for range 10 {
		go func() {
			defer wg.Done()
			r, e := Do("test", testcall)
			if e != nil {
				panic(e)
			}
			if *(*string)(r) != "test" {
				panic("return data error")
			}
		}()
	}
	wg.Wait()
}
func testcall() (unsafe.Pointer, error) {
	fmt.Println("testcall")
	time.Sleep(time.Second)
	t := "test"
	return unsafe.Pointer(&t), nil
}
