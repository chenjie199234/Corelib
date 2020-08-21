package first

import (
	//std
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	//third
	"github.com/chenjie199234/Corelib/web"
	"google.golang.org/protobuf/proto"

	//self
	"protoc-gen-web/second"
)

var webTestInstance webTest

type webTest interface {
	RegisterMidware() map[string][]web.OutsideHandler
	Hello(*web.Context, *HelloReq) (*HelloResp, error)
	Kiss(*web.Context, *second.KissReq) (*second.KissResp, error)
	Bye(*web.Context, *second.ByeReq) (*second.ByeResp, error)
}

func dealwebTestHello(ctx *web.Context) {
	req := &HelloReq{}
	switch ctx.GetContentType() {
	case "":
		fallthrough
	case "application/x-www-form-urlencoded":
		encoded := false
		if data := ctx.GetForm("encode"); data == "1" {
			encoded = true
		}
		if data := ctx.GetForm("json"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = json.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		} else if data := ctx.GetForm("proto"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = proto.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	case "application/json":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := json.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		}
	case "application/x-protobuf":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := proto.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	default:
		ctx.WriteString(http.StatusBadRequest, "unknown Content-Type:"+ctx.GetContentType())
		return
	}
	resp, e := webTestInstance.Hello(ctx, req)
	if e != nil {
		ctx.WriteString(http.StatusInternalServerError, e.Error())
		return
	}
	if strings.Contains(ctx.GetAcceptType(), "application/x-protobuf") {
		data, e := proto.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode proto response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	} else {
		data, e := json.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode json response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	}
}
func dealwebTestKiss(ctx *web.Context) {
	req := &second.KissReq{}
	switch ctx.GetContentType() {
	case "":
		fallthrough
	case "application/x-www-form-urlencoded":
		encoded := false
		if data := ctx.GetForm("encode"); data == "1" {
			encoded = true
		}
		if data := ctx.GetForm("json"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = json.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		} else if data := ctx.GetForm("proto"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = proto.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	case "application/json":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := json.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		}
	case "application/x-protobuf":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := proto.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	default:
		ctx.WriteString(http.StatusBadRequest, "unknown Content-Type:"+ctx.GetContentType())
		return
	}
	resp, e := webTestInstance.Kiss(ctx, req)
	if e != nil {
		ctx.WriteString(http.StatusInternalServerError, e.Error())
		return
	}
	if strings.Contains(ctx.GetAcceptType(), "application/x-protobuf") {
		data, e := proto.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode proto response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	} else {
		data, e := json.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode json response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	}
}
func dealwebTestBye(ctx *web.Context) {
	req := &second.ByeReq{}
	switch ctx.GetContentType() {
	case "":
		fallthrough
	case "application/x-www-form-urlencoded":
		encoded := false
		if data := ctx.GetForm("encode"); data == "1" {
			encoded = true
		}
		if data := ctx.GetForm("json"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = json.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		} else if data := ctx.GetForm("proto"); data != "" {
			var e error
			if encoded {
				if data, e = url.QueryUnescape(data); e != nil {
					ctx.WriteString(http.StatusBadRequest, "encoded request data format error:"+e.Error())
					return
				}
			}
			if e = proto.Unmarshal(web.Str2byte(data), req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	case "application/json":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := json.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode json request data error:"+e.Error())
				return
			}
		}
	case "application/x-protobuf":
		if body, _ := ctx.GetBody(); len(body) > 0 {
			if e := proto.Unmarshal(body, req); e != nil {
				ctx.WriteString(http.StatusBadRequest, "decode proto request data error:"+e.Error())
				return
			}
		}
	default:
		ctx.WriteString(http.StatusBadRequest, "unknown Content-Type:"+ctx.GetContentType())
		return
	}
	resp, e := webTestInstance.Bye(ctx, req)
	if e != nil {
		ctx.WriteString(http.StatusInternalServerError, e.Error())
		return
	}
	if strings.Contains(ctx.GetAcceptType(), "application/x-protobuf") {
		data, e := proto.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode proto response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	} else {
		data, e := json.Marshal(resp)
		if e != nil {
			ctx.WriteString(http.StatusInternalServerError, "encode json response data error:"+e.Error())
			return
		}
		ctx.Write(http.StatusOK, data)
	}
}

var PathTestHello = "/Test/Hello"
var PathTestKiss = "/Test/Kiss"
var PathTestBye = "/Test/Bye"

func RegisterwebTest(engine *web.Web, instance webTest) {
	webTestInstance = instance
	pathmids := instance.RegisterMidware()
	if mids, ok := pathmids[PathTestHello]; ok && len(mids) > 0 {
		engine.GET(PathTestHello, append(mids, dealwebTestHello)...)
	} else {
		engine.GET(PathTestHello, dealwebTestHello)
	}
	if mids, ok := pathmids[PathTestKiss]; ok && len(mids) > 0 {
		engine.POST(PathTestKiss, append(mids, dealwebTestKiss)...)
	} else {
		engine.POST(PathTestKiss, dealwebTestKiss)
	}
	if mids, ok := pathmids[PathTestBye]; ok && len(mids) > 0 {
		engine.DELETE(PathTestBye, append(mids, dealwebTestBye)...)
	} else {
		engine.DELETE(PathTestBye, dealwebTestBye)
	}
}
