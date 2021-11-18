package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/julienschmidt/httprouter"
)

func (this *WebServer) getContext(w http.ResponseWriter, r *http.Request, c context.Context, metadata map[string]string, handlers []OutsideHandler) *Context {
	ctx, ok := this.ctxpool.Get().(*Context)
	if !ok {
		return &Context{
			Context:  c,
			w:        w,
			r:        r,
			metadata: metadata,
			handlers: handlers,
			status:   0,
			e:        nil,
		}
	}
	ctx.Context = c
	ctx.w = w
	ctx.r = r
	ctx.metadata = metadata
	ctx.handlers = handlers
	ctx.status = 0
	ctx.e = nil
	return ctx
}

func (this *WebServer) putContext(ctx *Context) {
	this.ctxpool.Put(ctx)
}

type Context struct {
	context.Context
	w        http.ResponseWriter
	r        *http.Request
	metadata map[string]string
	handlers []OutsideHandler
	status   int8
	e        *cerror.Error
}

func (this *Context) run() {
	for _, handler := range this.handlers {
		handler(this)
		if this.status != 0 {
			break
		}
	}
}

//has race
func (this *Context) Abort(e error) {
	this.status = -1
	this.e = cerror.ConvertStdError(e)
	if this.e != nil {
		this.w.Header().Set("Content-Type", "application/json")
		if this.e.Httpcode < 400 || this.e.Httpcode > 999 {
			panic("[web.Context.Abort] httpcode must in [400,999]")
		}
		this.w.WriteHeader(int(this.e.Httpcode))
		this.w.Write(common.Str2byte(this.e.Error()))
	}
}

//has race
func (this *Context) Write(contenttype string, msg []byte) {
	this.status = 1
	this.w.WriteHeader(http.StatusOK)
	this.w.Header().Set("Content-Type", contenttype)
	this.w.Write(msg)
}

func (this *Context) WriteString(contenttype, msg string) {
	this.Write(contenttype, common.Str2byte(msg))
}

func (this *Context) SetHeader(k, v string) {
	this.w.Header().Set(k, v)
}
func (this *Context) AddHeader(k, v string) {
	this.w.Header().Add(k, v)
}
func (this *Context) GetRequest() *http.Request {
	return this.r
}
func (this *Context) GetResponse() http.ResponseWriter {
	return this.w
}
func (this *Context) GetHost() string {
	return this.r.URL.Host
}
func (this *Context) GetPath() string {
	return this.r.URL.Path
}
func (this *Context) GetMethod() string {
	return this.r.Method
}
func (this *Context) GetMetadata() map[string]string {
	return this.metadata
}
func (this *Context) GetHeaders() http.Header {
	return this.r.Header
}
func (this *Context) GetHeader(key string) string {
	return this.r.Header.Get(key)
}
func (this *Context) GetPeerName() string {
	peername := this.r.Header.Get("SourceApp")
	if peername == "" {
		return "unknown"
	}
	return peername
}
func (this *Context) GetPeerAddr() string {
	return this.r.RemoteAddr
}
func (this *Context) GetClientIp() string {
	return getclientip(this.r)
}
func getclientip(r *http.Request) string {
	ip := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
		if ip != "" {
			return ip
		}
	}
	if ip = strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip == "" {
		ip, _, _ = net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	}
	return ip
}
func (this *Context) GetUserAgent() string {
	return this.r.Header.Get("User-Agent")
}
func (this *Context) GetReferer() string {
	return this.r.Header.Get("Referer")
}
func (this *Context) GetContentType() string {
	return this.r.Header.Get("Content-Type")
}
func (this *Context) GetContentLanguage() string {
	return this.r.Header.Get("Content-Language")
}
func (this *Context) GetContentLength() int64 {
	return this.r.ContentLength
}
func (this *Context) GetAcceptType() string {
	return this.r.Header.Get("Accept")
}
func (this *Context) GetAcceptEncoding() string {
	return this.r.Header.Get("Accept-Encoding")
}
func (this *Context) GetAcceptLanguage() string {
	return this.r.Header.Get("Accept-Language")
}
func (this *Context) GetCookies() []*http.Cookie {
	return this.r.Cookies()
}
func (this *Context) GetCookie(key string) *http.Cookie {
	result, e := this.r.Cookie(key)
	if e == http.ErrNoCookie {
		return nil
	}
	return result
}
func (this *Context) ParseForm() error {
	if e := this.r.ParseForm(); e != nil {
		return e
	}
	if e := this.r.ParseMultipartForm(32 << 20); e != nil && e != http.ErrNotMultipart {
		return e
	}
	return nil
}

//must call ParseForm before this
func (this *Context) GetForm(key string) string {
	if len(this.r.Form) == 0 {
		return ""
	}
	return this.r.Form.Get(key)
}
func (this *Context) GetBody() ([]byte, error) {
	return io.ReadAll(this.r.Body)
}

//param is the value in dynamic url,see httprouter's dynamic path
//https://github.com/julienschmidt/httprouter
func (this *Context) GetParams() httprouter.Params {
	return httprouter.ParamsFromContext(this.Context)
}

//param is the value in dynamic url,see httprouter's dynamic path
//https://github.com/julienschmidt/httprouter
func (this *Context) GetParam(key string) string {
	return httprouter.ParamsFromContext(this.Context).ByName(key)
}
