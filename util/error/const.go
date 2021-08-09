package error

var (
	ErrReq    = &Error{Code: 10000, Msg: "request error"}
	ErrResp   = &Error{Code: 10001, Msg: "response error"}
	ErrNoMids = &Error{Code: 10002, Msg: "midware missing"}
)
