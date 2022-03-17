package mids

import (
	"net/http"

	cerror "github.com/chenjie199234/Corelib/error"
	publicmids "github.com/chenjie199234/Corelib/mids"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/web"
)

//dosn't include global mids in here
var all map[string]web.OutsideHandler

func init() {
	all = make(map[string]web.OutsideHandler)
	//register here
	all["rate"] = rate
	all["accesskey"] = accesskey
	all["accesssign"] = accesssign
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
func accesssign(ctx *web.Context) {
	accesssign := ctx.GetHeader("Access-Sign")
	if accesssign == "" {
		md := ctx.GetMetadata()
		accesssign = md["Access-Sign"]
	}
	if accesssign == "" {
		ctx.Abort(cerror.ErrAuth)
		return
	}
	body, e := ctx.GetBody()
	if e != nil {
		ctx.Abort(cerror.ErrReq)
		return
	}
	signdata := ctx.GetRequest().URL.RawQuery + common.Byte2str(body)
	if !publicmids.AccessSign(ctx.GetPath(), signdata, accesssign) {
		ctx.Abort(cerror.ErrAuth)
	}
}
