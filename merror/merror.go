package merror

import (
	"encoding/json"
	"fmt"

	"github.com/chenjie199234/Corelib/common"
)

//if error was not in merror format,code will return -1,msg will use the origin error.Error()

type MError struct {
	Code int64
	Msg  string
}

func MakeError(code int64, msg string) *MError {
	return &MError{Code: code, Msg: msg}
}
func GetCodeFromErrorstr(e string) int64 {
	if e == "" {
		return 0
	}
	tempe := &MError{}
	if ee := json.Unmarshal(common.Str2byte(e), tempe); ee != nil {
		return -1
	}
	return tempe.Code
}
func GetCodeFromError(e error) int64 {
	if e == nil {
		return 0
	}
	tempe, ok := e.(*MError)
	if ok {
		return tempe.Code
	}
	tempe = &MError{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), tempe); ee != nil {
		return -1
	}
	return tempe.Code
}
func GetMsgFromErrorstr(e string) string {
	if e == "" {
		return ""
	}
	tempe := &MError{}
	if ee := json.Unmarshal(common.Str2byte(e), tempe); ee != nil {
		return e
	}
	return tempe.Msg
}
func GetMsgFromError(e error) string {
	if e == nil {
		return ""
	}
	tempe, ok := e.(*MError)
	if ok {
		return tempe.Msg
	}
	tempe = &MError{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), tempe); ee != nil {
		return e.Error()
	}
	return tempe.Msg
}
func ErrorstrToMError(e string) *MError {
	if e == "" {
		return nil
	}
	result := &MError{}
	if ee := json.Unmarshal(common.Str2byte(e), result); ee != nil {
		return &MError{Code: -1, Msg: e}
	}
	return result
}
func ErrorToMError(e error) *MError {
	if e == nil {
		return nil
	}
	result, ok := e.(*MError)
	if ok {
		return result
	}
	result = &MError{}
	if ee := json.Unmarshal(common.Str2byte(e.Error()), result); ee != nil {
		return &MError{Code: -1, Msg: e.Error()}
	}
	return result
}
func (this *MError) Error() string {
	return fmt.Sprintf(`{"code":%d,"msg":"%s"}`, this.Code, this.Msg)
}
