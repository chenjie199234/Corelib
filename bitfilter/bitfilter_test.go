package bitfilter

import (
	"testing"
)

func Test_filter(t *testing.T) {
	filter := New(1024)
	data := make([][]byte, 2)
	data[0] = []byte("12345")
	data[1] = []byte("abcde")
	filter.Add(data)
	if !filter.IsAdd(data[0]) {
		panic("data check error")
	}
	if !filter.IsAdd(data[1]) {
		panic("data check error")
	}
	datas, addnum := filter.GetAllFilterData()
	filter.Clear()
	newfilter := Rebuild(datas, addnum)
	if !newfilter.IsAdd(data[0]) {
		panic("data check error")
	}
	if !newfilter.IsAdd(data[1]) {
		panic("data check error")
	}
}
