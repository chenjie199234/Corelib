package cerror

import (
	"context"
	"net/http"
)

// system,start from 1000
var (
	ErrClosing    = &Error{Code: 1000, Httpcode: 449, Msg: "server is closing,retry this request"}
	ErrNoapi      = &Error{Code: 1001, Httpcode: http.StatusNotImplemented, Msg: "api not implement"}
	ErrPanic      = &Error{Code: 1002, Httpcode: http.StatusServiceUnavailable, Msg: "server panic"}
	ErrNoserver   = &Error{Code: 1003, Httpcode: http.StatusServiceUnavailable, Msg: "no servers"}
	ErrClosed     = &Error{Code: 1004, Httpcode: http.StatusInternalServerError, Msg: "connection closed"}
	ErrReqmsgLen  = &Error{Code: 1005, Httpcode: http.StatusBadRequest, Msg: "req msg too large"}
	ErrRespmsgLen = &Error{Code: 1006, Httpcode: http.StatusInternalServerError, Msg: "resp msg too large"}
)

// business,start from 10000
var (
	ErrUnknown    = &Error{Code: 10000, Httpcode: http.StatusInternalServerError, Msg: "unknown"}
	ErrReq        = &Error{Code: 10001, Httpcode: http.StatusBadRequest, Msg: "request error"}
	ErrResp       = &Error{Code: 10002, Httpcode: http.StatusInternalServerError, Msg: "response error"}
	ErrSystem     = &Error{Code: 10003, Httpcode: http.StatusInternalServerError, Msg: "system error"}
	ErrToken      = &Error{Code: 10004, Httpcode: http.StatusUnauthorized, Msg: "token wrong"}
	ErrSession    = &Error{Code: 10005, Httpcode: http.StatusUnauthorized, Msg: "session wrong"}
	ErrAccessKey  = &Error{Code: 10006, Httpcode: http.StatusUnauthorized, Msg: "access key wrong"}
	ErrAccessSign = &Error{Code: 10007, Httpcode: http.StatusUnauthorized, Msg: "access sign wrong"}
	ErrPermission = &Error{Code: 10008, Httpcode: http.StatusForbidden, Msg: "permission denie"}
	ErrTooFast    = &Error{Code: 10009, Httpcode: http.StatusForbidden, Msg: "too fast"}
	ErrBan        = &Error{Code: 10010, Httpcode: http.StatusForbidden, Msg: "ban"}
	ErrBusy       = &Error{Code: 10011, Httpcode: http.StatusServiceUnavailable, Msg: "busy"}
	ErrNotExist   = &Error{Code: 10012, Httpcode: http.StatusNotFound, Msg: "not exist"}
)

// convert std error,always -1
var (
	ErrDeadlineExceeded = &Error{Code: -1, Httpcode: http.StatusGatewayTimeout, Msg: context.DeadlineExceeded.Error()}
	ErrCanceled         = &Error{Code: -1, Httpcode: http.StatusRequestTimeout, Msg: context.Canceled.Error()}
)
