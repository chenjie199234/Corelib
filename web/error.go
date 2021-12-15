package web

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi    = &error.Error{Code: 1001, Httpcode: http.StatusNotImplemented, Msg: "[web] api not implement"}
	errClosing  = &error.Error{Code: 1002, Httpcode: http.StatusInternalServerError, Msg: "[web] server is closing"}
	ErrPanic    = &error.Error{Code: 1003, Httpcode: http.StatusServiceUnavailable, Msg: "[web] server panic"}
	ErrNoserver = &error.Error{Code: 1004, Httpcode: http.StatusServiceUnavailable, Msg: "[web] no servers"}
	ErrCors     = &error.Error{Code: 1005, Httpcode: http.StatusMethodNotAllowed, Msg: "[web] cors"}
)
