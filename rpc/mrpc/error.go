package mrpc

import (
	"github.com/chenjie199234/Corelib/merror"
)

const (
	ERRLARGE = iota + 1
	ERRNOAPI
	ERRREQUEST
	ERRRESPONSE
	ERRCTXCANCEL
	ERRNOSERVER
	ERRCLOSING
	ERRCLOSED
	ERRPANIC
)

var ERR = map[uint64]*merror.MError{
	ERRLARGE:     &merror.MError{Code: ERRLARGE, Msg: "msg too large"},
	ERRNOAPI:     &merror.MError{Code: ERRNOAPI, Msg: "api not implement"},
	ERRREQUEST:   &merror.MError{Code: ERRREQUEST, Msg: "request data error"},
	ERRRESPONSE:  &merror.MError{Code: ERRRESPONSE, Msg: "response data error"},
	ERRCTXCANCEL: &merror.MError{Code: ERRCTXCANCEL, Msg: "context canceled"},
	ERRNOSERVER:  &merror.MError{Code: ERRNOSERVER, Msg: "no servers connected"},
	ERRCLOSING:   &merror.MError{Code: ERRCLOSING, Msg: "connection is closing"},
	ERRCLOSED:    &merror.MError{Code: ERRCLOSED, Msg: "connection is closed"},
	ERRPANIC:     &merror.MError{Code: ERRPANIC, Msg: "server panic"},
}
