package error

import (
	"context"
	"net/http"
)

var (
	ErrUnknown  = &Error{Code: 10000, Httpcode: http.StatusInternalServerError, Msg: "unknown"}
	ErrReq      = &Error{Code: 10001, Httpcode: http.StatusBadRequest, Msg: "request error"}
	ErrResp     = &Error{Code: 10002, Httpcode: http.StatusInternalServerError, Msg: "response error"}
	ErrSystem   = &Error{Code: 10003, Httpcode: http.StatusInternalServerError, Msg: "system error"}
	ErrAuth     = &Error{Code: 10004, Httpcode: http.StatusUnauthorized, Msg: "auth error"}
	ErrLimit    = &Error{Code: 10005, Httpcode: http.StatusServiceUnavailable, Msg: "limit"}
	ErrBan      = &Error{Code: 10006, Httpcode: http.StatusForbidden, Msg: "ban"}
	ErrNotExist = &Error{Code: 10007, Httpcode: http.StatusNotFound, Msg: "not exist"}
)

//convert std error
var (
	ErrDeadlineExceeded = &Error{Code: -1, Httpcode: http.StatusGatewayTimeout, Msg: context.DeadlineExceeded.Error()}
	ErrCanceled         = &Error{Code: -1, Httpcode: http.StatusRequestTimeout, Msg: context.Canceled.Error()}
)
