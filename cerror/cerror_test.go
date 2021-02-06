package cerror

import (
	"fmt"
	"testing"
)

func Test_MError(t *testing.T) {
	me := &CError{
		Code: 100,
		Msg:  "test",
	}
	ee := toerror(me)
	if code := GetCodeFromError(ee); code != 100 {
		panic("code error")
	}
	if code := GetCodeFromErrorstr(ee.Error()); code != 100 {
		panic("code error")
	}
	if msg := GetMsgFromError(ee); msg != "test" {
		panic("msg error")
	}
	if msg := GetMsgFromErrorstr(ee.Error()); msg != "test" {
		panic("msg error")
	}
	if temp := ErrorToMError(ee); temp.Code != me.Code || temp.Msg != me.Msg {
		panic("translate error")
	}
	if temp := ErrorstrToMError(ee.Error()); temp.Code != me.Code || temp.Msg != me.Msg {
		panic("translate error")
	}
	eee := fmt.Errorf("test")
	if code := GetCodeFromError(eee); code != -1 {
		panic("code error")
	}
	if code := GetCodeFromErrorstr(eee.Error()); code != -1 {
		panic("code error")
	}
	if msg := GetMsgFromError(eee); msg != "test" {
		panic("msg error")
	}
	if msg := GetMsgFromErrorstr(eee.Error()); msg != "test" {
		panic("msg error")
	}
	if temp := ErrorToMError(eee); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
	if temp := ErrorstrToMError(eee.Error()); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
}
func toerror(e *CError) error {
	return e
}
