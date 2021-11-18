package crpc

import (
	"net/http"

	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRNOAPI        = &error.Error{Code: 2001, Httpcode: http.StatusNotFound, Msg: "[crpc] api not implement"}
	ERRCLOSING      = &error.Error{Code: 2002, Httpcode: http.StatusInternalServerError, Msg: "[crpc] server is closing"}
	ERRPANIC        = &error.Error{Code: 2003, Httpcode: http.StatusServiceUnavailable, Msg: "[crpc] server panic"}
	ERRNOSERVER     = &error.Error{Code: 2004, Httpcode: http.StatusServiceUnavailable, Msg: "[crpc] no servers"}
	ERRCLOSED       = &error.Error{Code: 2005, Httpcode: http.StatusInternalServerError, Msg: "[crpc] connection closed"}
	ERRREQMSGLARGE  = &error.Error{Code: 2006, Httpcode: http.StatusBadRequest, Msg: "[crpc] req msg too large"}
	ERRRESPMSGLARGE = &error.Error{Code: 2007, Httpcode: http.StatusInternalServerError, Msg: "[crpc] resp msg too large"}
)
