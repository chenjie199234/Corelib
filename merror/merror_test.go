package merror

import (
	"testing"
)

func Test_MError(t *testing.T) {
	me := &MError{
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
}
func toerror(e *MError) error {
	return e
}
