package crpc

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRUNKNOWN      = &error.Error{Code: 1001, Msg: "[crpc] unknown error"}
	ERRNOAPI        = &error.Error{Code: 1002, Msg: "[crpc] api not implement"}
	ERRCLOSING      = &error.Error{Code: 1003, Msg: "[crpc] server is closing"}
	ERRPANIC        = &error.Error{Code: 1004, Msg: "[crpc] server panic"}
	ERRNOSERVER     = &error.Error{Code: 1005, Msg: "[crpc] no servers"}
	ERRCLOSED       = &error.Error{Code: 1006, Msg: "[crpc] connection closed"}
	ERRREQMSGLARGE  = &error.Error{Code: 1007, Msg: "[crpc] req msg too large"}
	ERRRESPMSGLARGE = &error.Error{Code: 1008, Msg: "[crpc] resp msg too large"}
)
