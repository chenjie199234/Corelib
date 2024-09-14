package cerror

import (
	"context"
	"net/http"
)

// system,start from 1000
var (
	ErrServerClosing    = MakeCError(1000, http.StatusServiceUnavailable, "server is closing,retry this request")
	ErrClientClosing    = MakeCError(1001, http.StatusBadRequest, "using closed client")
	ErrTarget           = MakeCError(1002, http.StatusBadRequest, "wrong server,check the server group and name")
	ErrNoapi            = MakeCError(1003, http.StatusNotImplemented, "api not implement")
	ErrPanic            = MakeCError(1004, http.StatusServiceUnavailable, "panic")
	ErrNoserver         = MakeCError(1005, http.StatusServiceUnavailable, "no servers")
	ErrNoSpecificserver = MakeCError(1006, http.StatusServiceUnavailable, "no specific server")
	ErrDiscoverStopped  = MakeCError(1007, http.StatusBadRequest, "discover stopped")
	ErrClosed           = MakeCError(1008, http.StatusInternalServerError, "connection closed")
	ErrStreamReadClosed = MakeCError(1009, http.StatusInternalServerError, "stream read closed")
	ErrStreamSendClosed = MakeCError(1010, http.StatusInternalServerError, "stream send closed")
	ErrReqmsgLen        = MakeCError(1011, http.StatusBadRequest, "req msg too large")
	ErrRespmsgLen       = MakeCError(1012, http.StatusInternalServerError, "resp msg too large")

	ErrCors = MakeCError(2001, http.StatusForbidden, "Cors forbidden")
)
var (
	ErrDataConflict = MakeCError(9001, http.StatusInternalServerError, "data conflict")
	ErrDataBroken   = MakeCError(9002, http.StatusInternalServerError, "data broken")

	ErrDBDataConflict = MakeCError(9101, http.StatusInternalServerError, "db data conflict")
	ErrDBDataBroken   = MakeCError(9102, http.StatusInternalServerError, "db data broken")

	ErrCacheDataConflict = MakeCError(9201, http.StatusInternalServerError, "cache data conflict")
	ErrCacheDataBroken   = MakeCError(9202, http.StatusInternalServerError, "cache data broken")

	ErrMQDataBroken = MakeCError(9301, http.StatusInternalServerError, "message queue data broken")
)

// business,start from 10000
var (
	ErrUnknown        = MakeCError(10000, http.StatusInternalServerError, "unknown")
	ErrReq            = MakeCError(10001, http.StatusBadRequest, "request error")
	ErrResp           = MakeCError(10002, http.StatusInternalServerError, "response error")
	ErrSystem         = MakeCError(10003, http.StatusInternalServerError, "system error")
	ErrToken          = MakeCError(10004, http.StatusUnauthorized, "token wrong")
	ErrSession        = MakeCError(10005, http.StatusUnauthorized, "session wrong")
	ErrAccessKey      = MakeCError(10006, http.StatusUnauthorized, "access key wrong")
	ErrAccessSign     = MakeCError(10007, http.StatusUnauthorized, "access sign wrong")
	ErrPermission     = MakeCError(10008, http.StatusForbidden, "permission denie")
	ErrTooFast        = MakeCError(10009, http.StatusForbidden, "too fast")
	ErrBan            = MakeCError(10010, http.StatusForbidden, "ban")
	ErrBusy           = MakeCError(10011, http.StatusServiceUnavailable, "busy")
	ErrNotExist       = MakeCError(10012, http.StatusNotFound, "not exist")
	ErrPasswordWrong  = MakeCError(10013, http.StatusBadRequest, "password wrong")
	ErrPasswordLength = MakeCError(10014, http.StatusBadRequest, "password length must <=32")
)

// convert std error,always -1
var (
	ErrDeadlineExceeded = MakeCError(-1, http.StatusGatewayTimeout, context.DeadlineExceeded.Error())
	ErrCanceled         = MakeCError(-1, http.StatusRequestTimeout, context.Canceled.Error())
)
