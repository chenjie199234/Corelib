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

//thread unsafe
func RegMid(name string, handler cgrpc.OutsideHandler) {
	all[name] = handler
}
func rate(ctx *cgrpc.Context) {
	if !publicmids.GrpcRate(ctx.GetPath()) {
		ctx.Abort(cerror.ErrLimit)
	}
}
