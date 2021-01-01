package discovery

import (
	"bytes"
	"fmt"
	"testing"
)

func Test_Msg(t *testing.T) {
	testonline("", []byte{'b'})
	testonline("a", []byte{'b'})
	testonline("a", []byte{'b'})
	testoffline("a")
	testoffline("a")
}
func testonline(a string, b []byte) {
	data := makeOnlineMsg(a, b)
	result := []byte{msgonline}
	result = append(result, []byte(a)...)
	result = append(result, split)
	result = append(result, b...)
	if !bytes.Equal(data, result) {
		panic("make online msg error")
	}
	aa, bb, e := getOnlineMsg(data)
	if e != nil {
		panic("get online msg error:" + e.Error())
	}
	if a != aa || !bytes.Equal(b, bb) {
		panic(fmt.Sprintf("get online msg broken,a:%s,b:%s", aa, bb))
	}
}
func testoffline(a string) {
	data := makeOfflineMsg(a)
	result := []byte{msgoffline}
	result = append(result, []byte(a)...)
	if !bytes.Equal(data, result) {
		panic("make offline msg error")
	}
	aa, e := getOfflineMsg(data)
	if e != nil {
		panic("get offline msg error:" + e.Error())
	}
	if a != aa {
		panic("get offline msg broken")
	}
}
func testpush() {
	temp := make(map[string][]byte)
	temp["a"] = []byte{'b'}
	data := makePushMsg(temp)
	result := []byte{msgpush}
	result = append(result, 'a', split, 'b')
	if !bytes.Equal(data, result) {
		panic("make push msg error")
	}
	a, e := getPushMsg(data)
	if e != nil {
		panic("get push msg error:" + e.Error())
	}
	for k, v := range a {
		if k != "a" || !bytes.Equal(v, []byte{'b'}) {
			panic("get push msg broken")
		}
	}
}
