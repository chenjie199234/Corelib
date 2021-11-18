package mids

import (
	"github.com/chenjie199234/Corelib/grpc"
)

//dosn't include global mids in here
var all map[string]grpc.OutsideHandler

func init() {
	all = make(map[string]grpc.OutsideHandler)
	//register here
}

func AllMids() map[string]grpc.OutsideHandler {
	return all
}
