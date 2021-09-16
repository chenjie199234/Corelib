package error

var (
	ErrUnknown  = &Error{Code: 10000, Msg: "unknown"}
	ErrReq      = &Error{Code: 10001, Msg: "request error"}
	ErrResp     = &Error{Code: 10002, Msg: "response error"}
	ErrSystem   = &Error{Code: 10003, Msg: "system error"}
	ErrAuth     = &Error{Code: 10004, Msg: "auth error"}
	ErrLimit    = &Error{Code: 10005, Msg: "limit"}
	ErrBan      = &Error{Code: 10006, Msg: "ban"}
	ErrNotExist = &Error{Code: 10007, Msg: "not exist"}
)
