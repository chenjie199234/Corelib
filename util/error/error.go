package error

import (
	"encoding/json"

	"github.com/chenjie199234/Corelib/util/common"
)

//if error was not in this error's format,code will return -1,msg will use the origin error.Error()

type Error struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

func MakeError(code int64, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}
func GetCodeFromErrorstr(e string) int64 {
	if e == "" {
		return 0
	}
	tempe := &Error{}
	if ee := json.Unmarshal(common.Str2byte(e), tempe); ee != nil {
		return -1
	}
	return tempe.Code
}
func GetCodeFromStdError(e error) int64 {
	if e == nil {
		return 0
	}
	tempe, ok := e.(*Error)
	if ok {
		return tempe.Code
	}
	tempe = &Error{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), tempe); ee != nil {
		return -1
	}
	return tempe.Code
}
func GetMsgFromErrorstr(e string) string {
	if e == "" {
		return ""
	}
	tempe := &Error{}
	if ee := json.Unmarshal(common.Str2byte(e), tempe); ee != nil {
		return e
	}
	return tempe.Msg
}
func GetMsgFromStdError(e error) string {
	if e == nil {
		return ""
	}
	tempe, ok := e.(*Error)
	if ok {
		return tempe.Msg
	}
	tempe = &Error{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), tempe); ee != nil {
		return e.Error()
	}
	return tempe.Msg
}
func ErrorstrToError(e string) *Error {
	if e == "" {
		return nil
	}
	result := &Error{}
	if ee := json.Unmarshal(common.Str2byte(e), result); ee != nil {
		return &Error{Code: -1, Msg: e}
	}
	return result
}
func StdErrorToError(e error) *Error {
	if e == nil {
		return nil
	}
	result, ok := e.(*Error)
	if ok {
		return result
	}
	result = &Error{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), result); ee != nil {
		return &Error{Code: -1, Msg: e.Error()}
	}
	return result
}
func Equal(a, b error) bool {
	aa := StdErrorToError(a)
	bb := StdErrorToError(b)
	return aa.Code == bb.Code && aa.Msg == bb.Msg
}
func (this *Error) Error() string {
	if this == nil {
		return ""
	}
	d, _ := json.Marshal(this)
	return common.Byte2str(d)
}
func (this *Error) String() string {
	if this == nil {
		return ""
	}
	d, _ := json.Marshal(this)
	return common.Byte2str(d)
}
