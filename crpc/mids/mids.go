package mids

import (
	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/crpc"
	publicmids "github.com/chenjie199234/Corelib/mids"
)

// dosn't include global mids in here
var all map[string]crpc.OutsideHandler

func init() {
	all = make(map[string]crpc.OutsideHandler)
	//register here
	all["rate"] = rate
	all["accesskey"] = accesskey
	all["token"] = token
	all["session"] = session
}

func AllMids() map[string]crpc.OutsideHandler {
	return all
}

// thread unsafe
func RegMid(name string, handler crpc.OutsideHandler) {
	all[name] = handler
}
func rate(ctx *crpc.Context) {
	if pass := publicmids.CrpcRate(ctx, ctx.GetPath()); !pass {
		ctx.Abort(cerror.ErrBusy)
	}
}
func accesskey(ctx *crpc.Context) {
	md := ctx.GetMetadata()
	accesskey := md["Access-Key"]
	if accesskey == "" {
		ctx.Abort(cerror.ErrAccessKey)
		return
	}
	if !publicmids.AccessKeyCheck(ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrAccessKey)
	}
}
func token(ctx *crpc.Context) {
	md := ctx.GetMetadata()
	tokenstr := md["Authorization"]
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
func session(ctx *crpc.Context) {
	md := ctx.GetMetadata()
	userid := md["Session-UID"]
	sessionid := md["Session-SID"]
	if userid == "" || sessionid == "" {
		ctx.Abort(cerror.ErrSession)
		return
	}
	sessiondata, pass := publicmids.VerifySession(ctx, userid, sessionid)
	if !pass {
		ctx.Abort(cerror.ErrSession)
		return
	}
	md["Session-Data"] = sessiondata
}
