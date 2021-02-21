package error

import (
	"fmt"
	"testing"
)

func Test_MError(t *testing.T) {
	me := &Error{
		Code: 100,
		Msg:  "test",
	}
	ee := toerror(me)
	if code := GetCodeFromStdError(ee); code != 100 {
		panic("code error")
	}
	if code := GetCodeFromErrorstr(ee.Error()); code != 100 {
		panic("code error")
	}
	if msg := GetMsgFromStdError(ee); msg != "test" {
		panic("msg error")
	}
	if msg := GetMsgFromErrorstr(ee.Error()); msg != "test" {
		panic("msg error")
	}
	if temp := StdErrorToError(ee); temp.Code != me.Code || temp.Msg != me.Msg {
		panic("translate error")
	}
	if temp := ErrorstrToError(ee.Error()); temp.Code != me.Code || temp.Msg != me.Msg {
		panic("translate error")
	}
	eee := fmt.Errorf("test")
	if code := GetCodeFromStdError(eee); code != -1 {
		panic("code error")
	}
	if code := GetCodeFromErrorstr(eee.Error()); code != -1 {
		panic("code error")
	}
	if msg := GetMsgFromStdError(eee); msg != "test" {
		panic("msg error")
	}
	if msg := GetMsgFromErrorstr(eee.Error()); msg != "test" {
		panic("msg error")
	}
	if temp := StdErrorToError(eee); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
	if temp := ErrorstrToError(eee.Error()); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
}
func toerror(e *Error) error {
	return e
}
