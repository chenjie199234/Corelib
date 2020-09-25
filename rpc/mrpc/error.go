package mrpc

import (
	"fmt"
)

const (
	ERRNOAPI = iota
	ERRREQUEST
)

var ERRMESSAGE = map[uint64]string{
	ERRNOAPI:   "api not implement",
	ERRREQUEST: "request data error",
}

func Errmaker(code uint64, msg string) (*MsgErr, error) {
	if code < 1000 {
		return nil, fmt.Errorf("[Mrpc.Errmaker]using rpc system error code")
	}
	return &MsgErr{
		Code: code,
		Msg:  msg,
	}, nil
}
