package mids

import (
	"net/http"
	"os"

	cerror "github.com/chenjie199234/Corelib/error"
	publicmids "github.com/chenjie199234/Corelib/mids"
	"github.com/chenjie199234/Corelib/web"
)

//dosn't include global mids in here
var all map[string]web.OutsideHandler

func init() {
	all = make(map[string]web.OutsideHandler)
	//register here
	all["rate"] = rate
	all["accesskey"] = accesskey
	all["token"] = token
}

func AllMids() map[string]web.OutsideHandler {
	return all
}

//thread unsafe
func RegMid(name string, handler web.OutsideHandler) {
	all[name] = handler
}

func rate(ctx *web.Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		if !publicmids.HttpGetRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPost:
		if !publicmids.HttpPostRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPut:
		if !publicmids.HttpPutRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodPatch:
		if !publicmids.HttpPatchRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrBusy)
		}
	case http.MethodDelete:
		if !publicmids.HttpDelRate(ctx.GetPath()) {
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
	if !publicmids.AccessKey(ctx.GetPath(), accesskey) {
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
