package rpc

import (
	"github.com/chenjie199234/Corelib/util/error"
)

var (
	ERRUNKNOWN      = &error.Error{Code: 1, Msg: "[rpc] unknown error"}
	ERRNOAPI        = &error.Error{Code: 2, Msg: "[rpc] api not implement"}
	ERRREQMSGLARGE  = &error.Error{Code: 3, Msg: "[rpc] req msg too large"}
	ERRRESPMSGLARGE = &error.Error{Code: 4, Msg: "[rpc] resp msg too large"}
	ERRNOSERVER     = &error.Error{Code: 5, Msg: "[rpc] no servers"}
	ERRCLOSING      = &error.Error{Code: 6, Msg: "[rpc] connection is closing"}
	ERRCLOSED       = &error.Error{Code: 7, Msg: "[rpc] connection closed"}
	ERRPANIC        = &error.Error{Code: 8, Msg: "[rpc] server panic"}
)
