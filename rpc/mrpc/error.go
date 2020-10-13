package mrpc

import "fmt"

const (
	ERRLARGE = iota + 1
	ERRNOAPI
	ERRREQUEST
	ERRRESPONSE
	ERRCTXCANCEL
	ERRNOSERVER
	ERRCLOSING
	ERRCLOSED
	ERRPANIC
)

var ERRMESSAGE = map[uint64]string{
	ERRLARGE:     "msg too large",
	ERRNOAPI:     "api not implement",
	ERRREQUEST:   "request data error",
	ERRRESPONSE:  "response data error",
	ERRCTXCANCEL: "context canceled",
	ERRNOSERVER:  "no servers connected",
	ERRCLOSING:   "connection is closing",
	ERRCLOSED:    "connection is closed",
	ERRPANIC:     "server panic",
}

func Errmaker(code uint64, msg string) *MsgErr {
	return &MsgErr{
		Code: code,
		Msg:  msg,
	}
}
func (this *MsgErr) Error() string {
	return fmt.Sprintf("code:%d,msg:%s", this.Code, this.Msg)
}
