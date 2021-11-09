package crpc

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRUNKNOWN      = &error.Error{Code: 1, Msg: "[crpc] unknown error"}
	ERRNOAPI        = &error.Error{Code: 2, Msg: "[crpc] api not implement"}
	ERRCLOSING      = &error.Error{Code: 3, Msg: "[crpc] connection is closing"}
	ERRPANIC        = &error.Error{Code: 4, Msg: "[crpc] server panic"}
	ERRNOSERVER     = &error.Error{Code: 5, Msg: "[crpc] no servers"}
	ERRCLOSED       = &error.Error{Code: 6, Msg: "[crpc] connection closed"}
	ERRREQMSGLARGE  = &error.Error{Code: 7, Msg: "[crpc] req msg too large"}
	ERRRESPMSGLARGE = &error.Error{Code: 8, Msg: "[crpc] resp msg too large"}
)
