package mrpc

import (
	"fmt"
)

const (
	ERRDATA = iota
	ERRNOAPI
)

var ERRMESSAGE = map[uint64]string{
	ERRDATA:  "rpc system data error",
	ERRNOAPI: "api not implement",
}

func Errmaker(code uint64, msg string) (*MsgErr, error) {
	if code < 1000 {
		return nil, fmt.Errorf("[Mrpc.Errmaker]using system error code")
	}
	return &MsgErr{
		Code: code,
		Msg:  msg,
	}, nil
}
