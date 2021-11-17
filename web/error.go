package web

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi    = &error.Error{Code: 1001, Msg: "[web] api not implement"}
	ErrClosing  = &error.Error{Code: 1002, Msg: "[web] server is closing"}
	ErrPanic    = &error.Error{Code: 1003, Msg: "[web] server panic"}
	ErrNoserver = &error.Error{Code: 1004, Msg: "[web] no servers"}
	ErrCors     = &error.Error{Code: 1005, Msg: "[web] cors"}
)
var (
	_ErrNoapiStr    = ErrNoapi.Error()
	_ErrClosingStr  = ErrClosing.Error()
	_ErrPanicStr    = ErrPanic.Error()
	_ErrNoserverStr = ErrNoserver.Error()
	_ErrCorsStr     = ErrCors.Error()
)

var (
	//_ErrUnknownStr  = error.ErrUnknown.Error()
	_ErrReqStr  = error.ErrReq.Error()
	_ErrRespStr = error.ErrResp.Error()
	//_ErrSystemStr   = error.ErrSystem.Error()
	//_ErrAuthStr     = error.ErrAuth.Error()
	//_ErrLimitStr    = error.ErrLimit.Error()
	//_ErrBanStr      = error.ErrBan.Error()
	//_ErrNotExistStr = error.ErrNotExist.Error()
)
var (
	_ErrDeadlineExceededStr = error.ErrDeadlineExceeded.Error()
	_ErrCanceledStr         = error.ErrCanceled.Error()
)
