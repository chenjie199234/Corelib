package mrpc

const (
	ERRLARGE = iota
	ERRNOAPI
	ERRREQUEST
	ERRCLOSING
	ERRCTXCANCEL
)

var ERRMESSAGE = map[uint64]string{
	ERRLARGE:     "msg too large",
	ERRNOAPI:     "api not implement",
	ERRREQUEST:   "request data error",
	ERRCLOSING:   "connection is closed",
	ERRCTXCANCEL: "context canceled",
}

func Errmaker(code uint64, msg string) *MsgErr {
	return &MsgErr{
		Code: code,
		Msg:  msg,
	}
}
