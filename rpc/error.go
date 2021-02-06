package rpc

import (
	"github.com/chenjie199234/Corelib/merror"
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

var ERR = map[uint64]*merror.MError{
	ERRUNKNOWN:    &merror.MError{Code: ERRUNKNOWN, Msg: "mrpc:unknown error"},
	ERRLARGE:      &merror.MError{Code: ERRLARGE, Msg: "mrpc:msg too large"},
	ERRNOAPI:      &merror.MError{Code: ERRNOAPI, Msg: "mrpc:api not implement"},
	ERRREQUEST:    &merror.MError{Code: ERRREQUEST, Msg: "mrpc:request data error"},
	ERRRESPONSE:   &merror.MError{Code: ERRRESPONSE, Msg: "mrpc:response data error"},
	ERRCTXCANCEL:  &merror.MError{Code: ERRCTXCANCEL, Msg: "mrpc:context canceled"},
	ERRCTXTIMEOUT: &merror.MError{Code: ERRCTXTIMEOUT, Msg: "mrpc:context timeout"},
	ERRNOSERVER:   &merror.MError{Code: ERRNOSERVER, Msg: "mrpc:no servers connected"},
	ERRCLOSING:    &merror.MError{Code: ERRCLOSING, Msg: "mrpc:connection is closing"},
	ERRCLOSED:     &merror.MError{Code: ERRCLOSED, Msg: "mrpc:connection is closed"},
	ERRPANIC:      &merror.MError{Code: ERRPANIC, Msg: "mrpc:server panic"},
}
