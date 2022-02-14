package mids

import (
	"github.com/chenjie199234/Corelib/crpc"
	cerror "github.com/chenjie199234/Corelib/error"
	publicmids "github.com/chenjie199234/Corelib/mids"
)

//dosn't include global mids in here
var all map[string]crpc.OutsideHandler

func init() {
	all = make(map[string]crpc.OutsideHandler)
	//register here
	all["rate"] = rate
	all["access"] = access
}

func AllMids() map[string]crpc.OutsideHandler {
	return all
}

//thread unsafe
func RegMid(name string, handler crpc.OutsideHandler) {
	all[name] = handler
}

func rate(ctx *crpc.Context) {
	if !publicmids.CrpcRate(ctx.GetPath()) {
		ctx.Abort(cerror.ErrLimit)
	}
}
func access(ctx *crpc.Context) {
	md := ctx.GetMetadata()
	accessid := md["Access-Id"]
	accesskey := md["Access-Key"]
	if accessid == "" {
		ctx.Abort(cerror.ErrAuth)
		return
	}
	if !publicmids.Access(accessid, accesskey) {
		ctx.Abort(cerror.ErrAuth)
	}
}
