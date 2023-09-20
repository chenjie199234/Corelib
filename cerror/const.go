package cerror

import (
	"context"
	"net/http"
)

// system,start from 1000
var (
	ErrServerClosing   = &Error{Code: 1000, Httpcode: 449, Msg: "server is closing,retry this request"}
	ErrClientClosing   = &Error{Code: 1001, Httpcode: http.StatusServiceUnavailable, Msg: "using closed clinet"}
	ErrTarget          = &Error{Code: 1002, Httpcode: http.StatusServiceUnavailable, Msg: "wrong server,check the server group and name"}
	ErrNoapi           = &Error{Code: 1003, Httpcode: http.StatusNotImplemented, Msg: "api not implement"}
	ErrPanic           = &Error{Code: 1004, Httpcode: http.StatusServiceUnavailable, Msg: "server panic"}
	ErrNoserver        = &Error{Code: 1005, Httpcode: http.StatusServiceUnavailable, Msg: "no servers"}
	ErrDiscoverStopped = &Error{Code: 1006, Httpcode: http.StatusInternalServerError, Msg: "discover stopped"}
	ErrClosed          = &Error{Code: 1007, Httpcode: http.StatusInternalServerError, Msg: "connection closed"}
	ErrReqmsgLen       = &Error{Code: 1008, Httpcode: http.StatusBadRequest, Msg: "req msg too large"}
	ErrRespmsgLen      = &Error{Code: 1009, Httpcode: http.StatusInternalServerError, Msg: "resp msg too large"}

	ErrCors = &Error{Code: 2001, Httpcode: http.StatusForbidden, Msg: "Cors forbidden"}
)
var (
	ErrDBDataConflict = &Error{Code: 9001, Httpcode: http.StatusInternalServerError, Msg: "db data conflict"}
	ErrDBDataBroken   = &Error{Code: 9002, Httpcode: http.StatusInternalServerError, Msg: "db data broken"}

	ErrCacheDataConflict = &Error{Code: 9101, Httpcode: http.StatusInternalServerError, Msg: "cache data conflict"}
	ErrCacheDataBroken   = &Error{Code: 9102, Httpcode: http.StatusInternalServerError, Msg: "cache data broken"}

	ErrMQDataBroken = &Error{Code: 9201, Httpcode: http.StatusInternalServerError, Msg: "message queue data broken"}
)

// business,start from 10000
var (
	ErrUnknown    = &Error{Code: 10000, Httpcode: http.StatusInternalServerError, Msg: "unknown"}
	ErrReq        = &Error{Code: 10001, Httpcode: http.StatusBadRequest, Msg: "request error"}
	ErrResp       = &Error{Code: 10002, Httpcode: http.StatusInternalServerError, Msg: "response error"}
	ErrSystem     = &Error{Code: 10003, Httpcode: http.StatusInternalServerError, Msg: "system error"}
	ErrToken      = &Error{Code: 10004, Httpcode: http.StatusUnauthorized, Msg: "token wrong"}
	ErrSession    = &Error{Code: 10005, Httpcode: http.StatusUnauthorized, Msg: "session wrong"}
	ErrKey        = &Error{Code: 10006, Httpcode: http.StatusUnauthorized, Msg: "key wrong"}
	ErrSign       = &Error{Code: 10007, Httpcode: http.StatusUnauthorized, Msg: "sign wrong"}
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
