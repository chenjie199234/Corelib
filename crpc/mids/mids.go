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
	all["accesskey"] = accesskey
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
func accesskey(ctx *crpc.Context) {
	md := ctx.GetMetadata()
	accesskey := md["Access-Key"]
	if accesskey == "" {
		ctx.Abort(cerror.ErrAuth)
		return
	}
	if !publicmids.AccessKey(ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrAuth)
	}
}
