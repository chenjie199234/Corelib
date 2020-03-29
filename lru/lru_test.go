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
	debug(l)
	data := l.Get("11")
	if data == nil{
		fmt.Println("nil")
	}else{
		panic("should be bil")
	}
	debug(l)
	data = l.Get("18")
	if data == nil{
		panic("should not be nil")
	}else{
		fmt.Println(*(*string)(data))
	}
	debug(l)
}

func debug(l *LruCache) {
	temp := l.head
	fmt.Println("len:",l.curcap)
	for temp != nil{
		fmt.Printf("key:%s,value:%s,ttl:%d\n",temp.data.key,*(*string)(temp.data.value),temp.data.ttl)
		temp = temp.next
	}
}
