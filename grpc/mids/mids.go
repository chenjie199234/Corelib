package mids

import (
	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/grpc"
	sharemids "github.com/chenjie199234/Corelib/mids"
)

//dosn't include global mids in here
var all map[string]grpc.OutsideHandler

func init() {
	all = make(map[string]grpc.OutsideHandler)
	//register here
	all["rate"] = rate
}

func AllMids() map[string]grpc.OutsideHandler {
	return all
}

func rate(ctx *grpc.Context) {
	if !sharemids.GrpcRate(ctx.GetPath()) {
		ctx.Abort(cerror.ErrLimit)
	}
}
