package rpc

import (
	"github.com/chenjie199234/Corelib/error"
)

const (
	ERRUNKNOWN = iota + 1
	ERRMSGLARGE
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

var ERR = map[uint64]*error.Error{
	ERRUNKNOWN:    &error.Error{Code: ERRUNKNOWN, Msg: "rpc:unknown error"},
	ERRMSGLARGE:   &error.Error{Code: ERRMSGLARGE, Msg: "rpc:msg too large"},
	ERRNOAPI:      &error.Error{Code: ERRNOAPI, Msg: "rpc:api not implement"},
	ERRREQUEST:    &error.Error{Code: ERRREQUEST, Msg: "rpc:request data error"},
	ERRRESPONSE:   &error.Error{Code: ERRRESPONSE, Msg: "rpc:response data error"},
	ERRCTXCANCEL:  &error.Error{Code: ERRCTXCANCEL, Msg: "rpc:context canceled"},
	ERRCTXTIMEOUT: &error.Error{Code: ERRCTXTIMEOUT, Msg: "rpc:context timeout"},
	ERRNOSERVER:   &error.Error{Code: ERRNOSERVER, Msg: "rpc:no servers connected"},
	ERRCLOSING:    &error.Error{Code: ERRCLOSING, Msg: "rpc:connection is closing"},
	ERRCLOSED:     &error.Error{Code: ERRCLOSED, Msg: "rpc:connection is closed"},
	ERRPANIC:      &error.Error{Code: ERRPANIC, Msg: "rpc:server panic"},
}
