package web

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi    = &error.Error{Code: 1001, Httpcode: http.StatusNotFound, Msg: "[web] api not implement"}
	errClosing  = &error.Error{Code: 1002, Httpcode: http.StatusInternalServerError, Msg: "[web] server is closing"}
	ErrPanic    = &error.Error{Code: 1003, Httpcode: http.StatusServiceUnavailable, Msg: "[web] server panic"}
	ErrNoserver = &error.Error{Code: 1004, Httpcode: http.StatusServiceUnavailable, Msg: "[web] no servers"}
	ErrCors     = &error.Error{Code: 1005, Httpcode: http.StatusMethodNotAllowed, Msg: "[web] cors"}
)
var (
	_ErrNoapiStr    = ErrNoapi.Error()
	_ErrClosingStr  = errClosing.Error()
	_ErrPanicStr    = ErrPanic.Error()
	_ErrNoserverStr = ErrNoserver.Error()
	_ErrCorsStr     = ErrCors.Error()
)

var (
	//_ErrUnknownStr  = error.ErrUnknown.Error()
	_ErrReqStr = error.ErrReq.Error()
	//_ErrRespStr = error.ErrResp.Error()
	//_ErrSystemStr   = error.ErrSystem.Error()
	//_ErrAuthStr     = error.ErrAuth.Error()
	//_ErrLimitStr    = error.ErrLimit.Error()
	//_ErrBanStr      = error.ErrBan.Error()
	//_ErrNotExistStr = error.ErrNotExist.Error()
)
var (
	_ErrDeadlineExceededStr = error.ErrDeadlineExceeded.Error()
	//_ErrCanceledStr         = error.ErrCanceled.Error()
)
