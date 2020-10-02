package mrpc

const (
	ERRLARGE = iota
	ERRNOAPI
	ERRREQUEST
	ERRRESPONSE
	ERRCLOSING
	ERRCTXCANCEL
	ERRNOSERVER
)

var ERRMESSAGE = map[uint64]string{
	ERRLARGE:     "msg too large",
	ERRNOAPI:     "api not implement",
	ERRREQUEST:   "request data error",
	ERRRESPONSE:  "response data error",
	ERRCLOSING:   "connection is closed",
	ERRCTXCANCEL: "context canceled",
	ERRNOSERVER:  "no servers connected",
}

func Errmaker(code uint64, msg string) *MsgErr {
	return &MsgErr{
		Code: code,
		Msg:  msg,
	}
}
