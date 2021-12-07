package mids

import (
	"github.com/chenjie199234/Corelib/cgrpc"
	cerror "github.com/chenjie199234/Corelib/error"
	publicmids "github.com/chenjie199234/Corelib/mids"
)

//dosn't include global mids in here
var all map[string]cgrpc.OutsideHandler

func init() {
	all = make(map[string]cgrpc.OutsideHandler)
	//register here
	all["rate"] = rate
}

func AllMids() map[string]cgrpc.OutsideHandler {
	return all
}

func rate(ctx *cgrpc.Context) {
	if !publicmids.GrpcRate(ctx.GetPath()) {
		ctx.Abort(cerror.ErrLimit)
	}
}
