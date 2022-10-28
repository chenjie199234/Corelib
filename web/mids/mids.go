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
	all["accesskey"] = accesskey
	all["token"] = token
	all["session"] = session
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
func accesskey(ctx *web.Context) {
	accesskey := ctx.GetHeader("Access-Key")
	md := ctx.GetMetadata()
	if accesskey == "" {
		accesskey = md["Access-Key"]
	} else {
		md["Access-Key"] = accesskey
	}
	if accesskey == "" {
		ctx.Abort(cerror.ErrAccessKey)
		return
	}
	if !publicmids.AccessKeyCheck(ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrAccessKey)
	}
}
func token(ctx *web.Context) {
	md := ctx.GetMetadata()
	tokenstr := ctx.GetHeader("Authorization")
	if tokenstr == "" {
		tokenstr = md["Authorization"]
	} else {
		md["Authorization"] = tokenstr
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
	userid := ctx.GetHeader("Session-UID")
	if userid == "" {
		userid = md["Session-UID"]
	} else {
		md["Session-UID"] = userid
	}
	if userid == "" {
		ctx.Abort(cerror.ErrSession)
		return
	}
	sessionid := ctx.GetHeader("Session-SID")
	if sessionid == "" {
		sessionid = md["Session-SID"]
	} else {
		md["Session-SID"] = sessionid
	}
	if sessionid == "" {
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
