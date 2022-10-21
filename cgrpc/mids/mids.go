package mids

import (
	"os"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cgrpc"
	publicmids "github.com/chenjie199234/Corelib/mids"
)

// dosn't include global mids in here
var all map[string]cgrpc.OutsideHandler

func init() {
	all = make(map[string]cgrpc.OutsideHandler)
	//register here
	all["rate"] = rate
	all["accesskey"] = accesskey
	all["token"] = token
}

func AllMids() map[string]cgrpc.OutsideHandler {
	return all
}

// thread unsafe
func RegMid(name string, handler cgrpc.OutsideHandler) {
	all[name] = handler
}
func rate(ctx *cgrpc.Context) {
	if !publicmids.GrpcRate(ctx.GetPath()) {
		ctx.Abort(cerror.ErrBusy)
	}
}
func accesskey(ctx *cgrpc.Context) {
	md := ctx.GetMetadata()
	accesskey := md["Access-Key"]
	if accesskey == "" {
		ctx.Abort(cerror.ErrAuth)
		return
	}
	if !publicmids.AccessKeyCheck(ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrAuth)
	}
}
func token(ctx *cgrpc.Context) {
	md := ctx.GetMetadata()
	tokenstr := md["Authorization"]
	secret := os.Getenv("TOKEN_SECRET")
	t, e := publicmids.VerifyToken(secret, tokenstr)
	if e != nil {
		ctx.Abort(e)
		return
	}
	md["Token-DeployEnv"] = t.DeployEnv
	md["Token-RunEnv"] = t.RunEnv
	md["Token-Puber"] = t.Puber
	md["Token-Data"] = t.Data
}
