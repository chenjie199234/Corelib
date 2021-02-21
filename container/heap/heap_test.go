package heap

import (
	"fmt"
	"testing"
	"unsafe"
)

func Test_Heap(t *testing.T) {
	maxnum := NewHeapMaxNum()
	var data int = 11001
	maxnum.Insert(1, unsafe.Pointer(&data))
	maxnum.Insert(3, unsafe.Pointer(&data))
	maxnum.Insert(4, unsafe.Pointer(&data))
	maxnum.Insert(6, unsafe.Pointer(&data))
	maxnum.Insert(7, unsafe.Pointer(&data))
	maxnum.Insert(10, unsafe.Pointer(&data))
	maxnum.Insert(9, unsafe.Pointer(&data))
	maxnum.Insert(8, unsafe.Pointer(&data))
	maxnum.Insert(5, unsafe.Pointer(&data))
	maxnum.Insert(2, unsafe.Pointer(&data))
	for i := int64(10); i >= 1; i-- {
		k, v := maxnum.Poproot()
		if k != i {
			panic(fmt.Sprintf("index error require:%d get:%d", i, k))
		}
		if (*(*int)(v)) != 11001 {
			panic("data error")
		}
	}
	minnum := NewHeapMinNum()
	minnum.Insert(1, unsafe.Pointer(&data))
	minnum.Insert(3, unsafe.Pointer(&data))
	minnum.Insert(4, unsafe.Pointer(&data))
	minnum.Insert(6, unsafe.Pointer(&data))
	minnum.Insert(7, unsafe.Pointer(&data))
	minnum.Insert(10, unsafe.Pointer(&data))
	minnum.Insert(9, unsafe.Pointer(&data))
	minnum.Insert(8, unsafe.Pointer(&data))
	minnum.Insert(5, unsafe.Pointer(&data))
	minnum.Insert(2, unsafe.Pointer(&data))
	for i := int64(1); i <= 10; i++ {
		k, v := minnum.Poproot()
		if k != i {
			panic(fmt.Sprintf("index error require:%d get:%d", i, k))
		}
		if (*(*int)(v)) != 11001 {
			panic("data error")
		}
	}
	maxstr := NewHeapMaxStr()
	maxstr.Insert("b", unsafe.Pointer(&data))
	maxstr.Insert("a", unsafe.Pointer(&data))
	maxstr.Insert("c", unsafe.Pointer(&data))
	maxstr.Insert("d", unsafe.Pointer(&data))
	maxstr.Insert("i", unsafe.Pointer(&data))
	maxstr.Insert("j", unsafe.Pointer(&data))
	maxstr.Insert("g", unsafe.Pointer(&data))
	maxstr.Insert("f", unsafe.Pointer(&data))
	maxstr.Insert("e", unsafe.Pointer(&data))
	maxstr.Insert("h", unsafe.Pointer(&data))
	for i := 'j'; i <= 'a'; i-- {
		k, v := maxstr.Poproot()
		if k != string(i) {
			panic(fmt.Sprintf("index error require:%v get:%s", i, k))
		}
		if (*(*int)(v)) != 11001 {
			panic("data error")
		}
	}
	minstr := NewHeapMinStr()
	minstr.Insert("b", unsafe.Pointer(&data))
	minstr.Insert("a", unsafe.Pointer(&data))
	minstr.Insert("c", unsafe.Pointer(&data))
	minstr.Insert("d", unsafe.Pointer(&data))
	minstr.Insert("i", unsafe.Pointer(&data))
	minstr.Insert("j", unsafe.Pointer(&data))
	minstr.Insert("g", unsafe.Pointer(&data))
	minstr.Insert("f", unsafe.Pointer(&data))
	minstr.Insert("e", unsafe.Pointer(&data))
	minstr.Insert("h", unsafe.Pointer(&data))
	for i := 'a'; i <= 'j'; i++ {
		k, v := minstr.Poproot()
		if k != string(i) {
			panic(fmt.Sprintf("index error require:%v get:%s", i, k))
		}
		if (*(*int)(v)) != 11001 {
			panic("data error")
		}
	}
}
