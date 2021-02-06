package rpc

import (
	"github.com/chenjie199234/Corelib/cerror"
)

const (
	ERRUNKNOWN = iota + 1
	ERRLARGE
	ERRNOAPI
	ERRREQUEST
	ERRRESPONSE
	ERRCTXCANCEL
	ERRCTXTIMEOUT
	ERRNOSERVER
	ERRCLOSING
	ERRCLOSED
	ERRPANIC
)

var ERR = map[uint64]*cerror.CError{
	ERRUNKNOWN:    &cerror.CError{Code: ERRUNKNOWN, Msg: "rpc:unknown error"},
	ERRLARGE:      &cerror.CError{Code: ERRLARGE, Msg: "rpc:msg too large"},
	ERRNOAPI:      &cerror.CError{Code: ERRNOAPI, Msg: "rpc:api not implement"},
	ERRREQUEST:    &cerror.CError{Code: ERRREQUEST, Msg: "rpc:request data error"},
	ERRRESPONSE:   &cerror.CError{Code: ERRRESPONSE, Msg: "rpc:response data error"},
	ERRCTXCANCEL:  &cerror.CError{Code: ERRCTXCANCEL, Msg: "rpc:context canceled"},
	ERRCTXTIMEOUT: &cerror.CError{Code: ERRCTXTIMEOUT, Msg: "rpc:context timeout"},
	ERRNOSERVER:   &cerror.CError{Code: ERRNOSERVER, Msg: "rpc:no servers connected"},
	ERRCLOSING:    &cerror.CError{Code: ERRCLOSING, Msg: "rpc:connection is closing"},
	ERRCLOSED:     &cerror.CError{Code: ERRCLOSED, Msg: "rpc:connection is closed"},
	ERRPANIC:      &cerror.CError{Code: ERRPANIC, Msg: "rpc:server panic"},
}
