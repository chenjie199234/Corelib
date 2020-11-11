package once

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func Test_Once(t *testing.T) {
	o := NewOnce()
	wg := new(sync.WaitGroup)
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			r, e := o.Do("test", testcall)
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
