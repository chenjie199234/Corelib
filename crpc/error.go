package crpc

import (
	"github.com/chenjie199234/Corelib/error"
)

var (
	ERRUNKNOWN      = &error.Error{Code: 2001, Msg: "[crpc] unknown error"}
	ERRNOAPI        = &error.Error{Code: 2002, Msg: "[crpc] api not implement"}
	ERRCLOSING      = &error.Error{Code: 2003, Msg: "[crpc] server is closing"}
	ERRPANIC        = &error.Error{Code: 2004, Msg: "[crpc] server panic"}
	ERRNOSERVER     = &error.Error{Code: 2005, Msg: "[crpc] no servers"}
	ERRCLOSED       = &error.Error{Code: 2006, Msg: "[crpc] connection closed"}
	ERRREQMSGLARGE  = &error.Error{Code: 2007, Msg: "[crpc] req msg too large"}
	ERRRESPMSGLARGE = &error.Error{Code: 2008, Msg: "[crpc] resp msg too large"}
)
