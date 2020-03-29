package lru

import (
	"testing"
	"unsafe"
	"fmt"
	"time"
)

func Test_lru(t *testing.T) {
	l:=New(10,1)
	for i:=0;i<20;i++{
		if i == 18 {
			time.Sleep(5*time.Second)
		}
		d:=fmt.Sprintf("%d",i)
		l.Set(d,unsafe.Pointer(&d))
	}
	l.debug()
	data := l.Get("11")
	if data == nil{
		fmt.Println("nil")
	}else{
		panic("should be bil")
	}
	l.debug()
	data = l.Get("18")
	if data == nil{
		panic("should not be nil")
	}else{
		fmt.Println(*(*string)(data))
	}
	l.debug()
}
