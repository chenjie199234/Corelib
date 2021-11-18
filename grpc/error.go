package grpc

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi    = &error.Error{Code: 3001, Httpcode: http.StatusNotFound, Msg: "[grpc] api not implement"}
	errClosing  = &error.Error{Code: 3002, Httpcode: http.StatusInternalServerError, Msg: "[grpc] server is closing"}
	ErrPanic    = &error.Error{Code: 3003, Httpcode: http.StatusServiceUnavailable, Msg: "[grpc] server panic"}
	ErrNoserver = &error.Error{Code: 3004, Httpcode: http.StatusServiceUnavailable, Msg: "[grpc] no servers"}
	ErrClosed   = &error.Error{Code: 3005, Httpcode: http.StatusInternalServerError, Msg: "[grpc] connection closed"}
)
