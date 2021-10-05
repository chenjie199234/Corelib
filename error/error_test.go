package error

import (
	"fmt"
	"testing"
)

func Test_Error(t *testing.T) {
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
	if temp := ConvertStdError(ee); temp.Code != me.Code || temp.Msg != me.Msg {
		panic("translate error")
	}
	if temp := ConvertErrorstr(ee.Error()); temp.Code != me.Code || temp.Msg != me.Msg {
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
	if temp := ConvertStdError(eee); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
	if temp := ConvertErrorstr(eee.Error()); temp.Code != -1 || temp.Msg != "test" {
		panic("translate error")
	}
	var nilerror *Error = nil
	notnilerror := toerror(nilerror)
	if notnilerror.Error() != "" {
		panic("nil error's string should be empty")
	}
}
func toerror(e *Error) error {
	return e
}
