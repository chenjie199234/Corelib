package mids

import (
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
	all["token"] = token
	all["session"] = session
	all["accesskey"] = accesskey
}

func AllMids() map[string]cgrpc.OutsideHandler {
	return all
}

// thread unsafe
func RegMid(name string, handler cgrpc.OutsideHandler) {
	all[name] = handler
}
func rate(ctx *cgrpc.Context) {
	if pass := publicmids.GrpcRate(ctx, ctx.GetPath()); !pass {
		ctx.Abort(cerror.ErrBusy)
	}
}
func token(ctx *cgrpc.Context) {
	md := ctx.GetMetadata()
	tokenstr := md["Token"]
	if tokenstr == "" {
		ctx.Abort(cerror.ErrToken)
		return
	}
	t := publicmids.VerifyToken(ctx, tokenstr)
	if t == nil {
		ctx.Abort(cerror.ErrToken)
		return
	}
	md["Token-DeployEnv"] = t.DeployEnv
	md["Token-RunEnv"] = t.RunEnv
	md["Token-Puber"] = t.Puber
	md["Token-Data"] = t.Data
}
func session(ctx *cgrpc.Context) {
	md := ctx.GetMetadata()
	sessionstr := md["Session"]
	if sessionstr == "" {
		ctx.Abort(cerror.ErrSession)
		return
	}
	sessiondata, pass := publicmids.VerifySession(ctx, sessionstr)
	if !pass {
		ctx.Abort(cerror.ErrSession)
		return
	}
	md["Session-Data"] = sessiondata
}
func accesskey(ctx *cgrpc.Context) {
	md := ctx.GetMetadata()
	accesskey := md["Access-Key"]
	if accesskey == "" {
		ctx.Abort(cerror.ErrKey)
		return
	}
	delete(md, "Access-Key")
	if !publicmids.VerifyAccessKey(ctx, "GRPC", ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrKey)
	}
}
