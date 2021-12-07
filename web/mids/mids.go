package mids

import (
	"net/http"

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
}

func AllMids() map[string]web.OutsideHandler {
	return all
}
func rate(ctx *web.Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		if !publicmids.HttpGetRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrLimit)
		}
	case http.MethodPost:
		if !publicmids.HttpPostRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrLimit)
		}
	case http.MethodPut:
		if !publicmids.HttpPutRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrLimit)
		}
	case http.MethodPatch:
		if !publicmids.HttpPatchRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrLimit)
		}
	case http.MethodDelete:
		if !publicmids.HttpDelRate(ctx.GetPath()) {
			ctx.Abort(cerror.ErrLimit)
		}
	default:
		ctx.Abort(cerror.ErrNotExist)
	}
}
