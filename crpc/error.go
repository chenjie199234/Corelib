package crpc

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ErrNoapi      = &error.Error{Code: 2001, Httpcode: http.StatusNotFound, Msg: "[crpc] api not implement"}
	errClosing    = &error.Error{Code: 2002, Httpcode: http.StatusInternalServerError, Msg: "[crpc] server is closing"}
	ErrPanic      = &error.Error{Code: 2003, Httpcode: http.StatusServiceUnavailable, Msg: "[crpc] server panic"}
	ErrNoserver   = &error.Error{Code: 2004, Httpcode: http.StatusServiceUnavailable, Msg: "[crpc] no servers"}
	ErrClosed     = &error.Error{Code: 2005, Httpcode: http.StatusInternalServerError, Msg: "[crpc] connection closed"}
	ErrReqmsgLen  = &error.Error{Code: 2006, Httpcode: http.StatusBadRequest, Msg: "[crpc] req msg too large"}
	ErrRespmsgLen = &error.Error{Code: 2007, Httpcode: http.StatusInternalServerError, Msg: "[crpc] resp msg too large"}
)
