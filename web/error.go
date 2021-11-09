package web

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRUNKNOWN  = &error.Error{Code: 1, Msg: "[web] unknown error"}
	ERRNOAPI    = &error.Error{Code: 2, Msg: "[web] api not implement"}
	ERRCLOSING  = &error.Error{Code: 3, Msg: "[web] connection is closing"}
	ERRPANIC    = &error.Error{Code: 4, Msg: "[web] server panic"}
	ERRNOSERVER = &error.Error{Code: 5, Msg: "[web] no servers"}
	ERRCLOSED   = &error.Error{Code: 6, Msg: "[web] connection closed"}
	ERRCORS     = &error.Error{Code: 7, Msg: "[web] cors"}
)
