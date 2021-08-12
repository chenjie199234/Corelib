package error

var (
	ErrUnknown = &Error{Code: 10000, Msg: "unknown"}
	ErrReq     = &Error{Code: 10001, Msg: "request error"}
	ErrResp    = &Error{Code: 10002, Msg: "response error"}
)
