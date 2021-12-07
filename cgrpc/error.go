package cgrpc

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi    = &error.Error{Code: 3001, Httpcode: http.StatusNotImplemented, Msg: "[cgrpc] api not implement"}
	errClosing  = &error.Error{Code: 3002, Httpcode: http.StatusInternalServerError, Msg: "[cgrpc] server is closing"}
	ErrPanic    = &error.Error{Code: 3003, Httpcode: http.StatusServiceUnavailable, Msg: "[cgrpc] server panic"}
	ErrNoserver = &error.Error{Code: 3004, Httpcode: http.StatusServiceUnavailable, Msg: "[cgrpc] no servers"}
	ErrClosed   = &error.Error{Code: 3005, Httpcode: http.StatusInternalServerError, Msg: "[cgrpc] connection closed"}
)
