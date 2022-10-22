package mids

import (
	"net/http"
	"os"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	publicmids "github.com/chenjie199234/Corelib/mids"
	"github.com/chenjie199234/Corelib/web"
)

// dosn't include global mids in here
var all map[string]web.OutsideHandler

func init() {
	all = make(map[string]web.OutsideHandler)
	//register here
	all["selfrate"] = selfrate
	all["globalrate"] = globalrate
	all["accesskey"] = accesskey
	all["token"] = token
}

func AllMids() map[string]web.OutsideHandler {
	return all
}

// thread unsafe
func RegMid(name string, handler web.OutsideHandler) {
	all[name] = handler
}
func selfrate(ctx *web.Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		if pass, _ := publicmids.HttpGetRate(ctx, ctx.GetPath(), false); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPost:
		if pass, _ := publicmids.HttpPostRate(ctx, ctx.GetPath(), false); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPut:
		if pass, _ := publicmids.HttpPutRate(ctx, ctx.GetPath(), false); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPatch:
		if pass, _ := publicmids.HttpPatchRate(ctx, ctx.GetPath(), false); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodDelete:
		if pass, _ := publicmids.HttpDelRate(ctx, ctx.GetPath(), false); !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	default:
		ctx.Abort(cerror.ErrNotExist)
	}
}
func globalrate(ctx *web.Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		if pass, e := publicmids.HttpGetRate(ctx, ctx.GetPath(), true); e != nil {
			log.Error(ctx, "[rate.global] path:", ctx.GetPath(), "method: GET", e)
			ctx.Abort(cerror.ErrBusy)
		} else if !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPost:
		if pass, e := publicmids.HttpPostRate(ctx, ctx.GetPath(), true); e != nil {
			log.Error(ctx, "[rate.global] path:", ctx.GetPath(), "method: POST", e)
			ctx.Abort(cerror.ErrBusy)
		} else if !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPut:
		if pass, e := publicmids.HttpPutRate(ctx, ctx.GetPath(), true); e != nil {
			log.Error(ctx, "[rate.global] path:", ctx.GetPath(), "method: PUT", e)
			ctx.Abort(cerror.ErrBusy)
		} else if !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPatch:
		if pass, e := publicmids.HttpPatchRate(ctx, ctx.GetPath(), true); e != nil {
			log.Error(ctx, "[rate.global] path:", ctx.GetPath(), "method: PATCH", e)
			ctx.Abort(cerror.ErrBusy)
		} else if !pass {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodDelete:
		if pass, e := publicmids.HttpDelRate(ctx, ctx.GetPath(), true); e != nil {
			log.Error(ctx, "[rate.global] path:", ctx.GetPath(), "method: DELETE", e)
			ctx.Abort(cerror.ErrBusy)
		} else if !pass {
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
		ctx.Abort(cerror.ErrAuth)
		return
	}
	if !publicmids.AccessKeyCheck(ctx.GetPath(), accesskey) {
		ctx.Abort(cerror.ErrAuth)
	}
}
func token(ctx *web.Context) {
	tokenstr := ctx.GetHeader("Authorization")
	secret := os.Getenv("TOKEN_SECRET")
	md := ctx.GetMetadata()
	if tokenstr == "" {
		tokenstr = md["Authorization"]
	} else {
		md["Authorization"] = tokenstr
	}
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
