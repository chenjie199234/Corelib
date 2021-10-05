package error

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/chenjie199234/Corelib/util/common"
)

//if error was not in this error's format,code will return -1,msg will use the origin error.Error()

func MakeError(code int32, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}
func GetCodeFromErrorstr(e string) int32 {
	ee := ConvertErrorstr(e)
	if ee == nil {
		return 0
	}
	return ee.Code
}
func GetCodeFromStdError(e error) int32 {
	ee := ConvertStdError(e)
	if ee == nil {
		return 0
	}
	return ee.Code
}
func GetMsgFromErrorstr(e string) string {
	ee := ConvertErrorstr(e)
	if ee == nil {
		return ""
	}
	return ee.Msg
}
func GetMsgFromStdError(e error) string {
	ee := ConvertStdError(e)
	if ee == nil {
		return ""
	}
	return ee.Msg
}
func ConvertErrorstr(e string) *Error {
	if e == "" {
		return nil
	}
	result := &Error{}
	if e[0] == '{' {
		//json format
		if ee := json.Unmarshal(common.Str2byte(e), result); ee != nil {
			result.Code = -1
			result.Msg = e
		}
	} else {
		//text format
		result.Code = -1
		result.Msg = e
	}
	return result
}
func ConvertStdError(e error) *Error {
	if e == nil {
		return nil
	}
	if e == context.DeadlineExceeded {
		return ErrDeadlineExceeded
	} else if e == context.Canceled {
		return ErrCanceled
	}
	result, ok := e.(*Error)
	if ok {
		return result
	}
	return ConvertErrorstr(e.Error())
}
func Equal(a, b error) bool {
	aa := ConvertStdError(a)
	bb := ConvertStdError(b)
	if aa == nil && bb == nil {
		return true
	} else if (aa == nil && bb != nil) || (aa != nil && bb == nil) {
		return false
	}
	return aa.Code == bb.Code && aa.Msg == bb.Msg
}
func (this *Error) Error() string {
	if this == nil {
		return ""
	}
	return "{\"code\":" + strconv.FormatInt(int64(this.Code), 10) + ",\"msg\":\"" + this.Msg + "\"}"
}
