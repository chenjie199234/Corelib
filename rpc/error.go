package rpc

import (
	"github.com/chenjie199234/Corelib/util/error"
)

var (
	ERRUNKNOWN    = &error.Error{Code: 1, Msg: "[rpc] unknown error"}
	ERRMSGLARGE   = &error.Error{Code: 2, Msg: "[rpc] msg too large"}
	ERRNOAPI      = &error.Error{Code: 3, Msg: "[rpc] api not implement"}
	ERRREQUEST    = &error.Error{Code: 4, Msg: "[rpc] request data error"}
	ERRRESPONSE   = &error.Error{Code: 5, Msg: "[rpc] response data error"}
	ERRCTXCANCEL  = &error.Error{Code: 6, Msg: "[rpc] context canceled"}
	ERRCTXTIMEOUT = &error.Error{Code: 7, Msg: "[rpc] context timeout"}
	ERRNOSERVER   = &error.Error{Code: 8, Msg: "[rpc] no servers"}
	ERRCLOSING    = &error.Error{Code: 9, Msg: "[rpc] connection is closing"}
	ERRCLOSED     = &error.Error{Code: 10, Msg: "[rpc] connection is closed"}
	ERRPANIC      = &error.Error{Code: 11, Msg: "[rpc] server panic"}
)
