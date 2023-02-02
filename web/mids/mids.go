package mids

import (
	"net/http"

	"github.com/chenjie199234/Corelib/cerror"
	publicmids "github.com/chenjie199234/Corelib/mids"
	"github.com/chenjie199234/Corelib/web"
)

// dosn't include global mids in here
var all map[string]web.OutsideHandler

func init() {
	all = make(map[string]web.OutsideHandler)
	//register here
	all["rate"] = rate
	all["token"] = token
	all["session"] = session
	all["accesskey"] = accesskey
}

func AllMids() map[string]web.OutsideHandler {
	return all
}

// thread unsafe
func RegMid(name string, handler web.OutsideHandler) {
	all[name] = handler
}
func rate(ctx *web.Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		if pass := publicmids.HttpGetRate(ctx, ctx.GetPath()); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPost:
		if pass := publicmids.HttpPostRate(ctx, ctx.GetPath()); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPut:
		if pass := publicmids.HttpPutRate(ctx, ctx.GetPath()); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPatch:
		if pass := publicmids.HttpPatchRate(ctx, ctx.GetPath()); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodDelete:
		if pass := publicmids.HttpDelRate(ctx, ctx.GetPath()); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	default:
		ctx.Abort(cerror.ErrNotExist)
	}
}
func token(ctx *web.Context) {
	md := ctx.GetMetadata()
	tokenstr := ctx.GetHeader("Token")
	if tokenstr == "" {
		tokenstr = md["Token"]
	} else {
		md["Token"] = tokenstr
	}
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
func session(ctx *web.Context) {
	md := ctx.GetMetadata()
	sessionstr := ctx.GetHeader("Session")
	if sessionstr == "" {
		sessionstr = md["Session"]
	} else {
		md["Session"] = sessionstr
	}
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
func accesskey(ctx *web.Context) {
	md := ctx.GetMetadata()
	accesskey := ctx.GetHeader("Access-Key")
	if accesskey == "" {
		accesskey = md["Access-Key"]
		delete(md, "Access-Key")
	}
	if accesskey == "" {
		ctx.Abort(cerror.ErrKey)
		return
	}
	if !publicmids.VerifyAccessKey(ctx, ctx.GetMethod(), ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrKey)
	}
}
