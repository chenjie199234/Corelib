package discovery

import (
	"bytes"
	"fmt"
	"testing"
)

func Test_Msg(t *testing.T) {
	testonline("", []byte{'b'}, nil)
	testonline("a", []byte{'b'}, nil)
	testonline("a", []byte{'b'}, []byte{'c'})
	testoffline("a", nil)
	testoffline("a", []byte{'b'})
}
func testonline(a string, b, c []byte) {
	data := makeOnlineMsg(a, b, c)
	result := []byte{mSGONLINE}
	result = append(result, []byte(a)...)
	result = append(result, sPLIT)
	result = append(result, b...)
	result = append(result, sPLIT)
	if len(c) != 0 {
		result = append(result, c...)
	}
	if !bytes.Equal(data, result) {
		panic("make online msg error")
	}
	aa, bb, cc, e := getOnlineMsg(data)
	if e != nil {
		panic("get online msg error:" + e.Error())
	}
	if a != aa || !bytes.Equal(b, bb) || !bytes.Equal(c, cc) {
		panic(fmt.Sprintf("get online msg broken,a:%s,b:%s,c:%s", aa, bb, cc))
	}
}
func testoffline(a string, b []byte) {
	data := makeOfflineMsg(a, b)
	result := []byte{mSGOFFLINE}
	result = append(result, []byte(a)...)
	result = append(result, sPLIT)
	if len(b) != 0 {
		result = append(result, b...)
	}
	if !bytes.Equal(data, result) {
		panic("make offline msg error")
	}
	aa, bb, e := getOfflineMsg(data)
	if e != nil {
		panic("get offline msg error:" + e.Error())
	}
	if a != aa || !bytes.Equal(b, bb) {
		panic("get offline msg broken")
	}
}
func testpush() {
	temp := make(map[string][]byte)
	temp["a"] = []byte{'b'}
	data := makePushMsg(temp)
	result := []byte{mSGPUSH}
	result = append(result, 'a', sPLIT, 'b')
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
