package web

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRUNKNOWN  = &error.Error{Code: 1001, Msg: "[web] unknown error"}
	ERRNOAPI    = &error.Error{Code: 1002, Msg: "[web] api not implement"}
	ERRCLOSING  = &error.Error{Code: 1003, Msg: "[web] server is closing"}
	ERRPANIC    = &error.Error{Code: 1004, Msg: "[web] server panic"}
	ERRNOSERVER = &error.Error{Code: 1005, Msg: "[web] no servers"}
)
