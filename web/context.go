package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"
)

func (s *WebServer) getContext(w http.ResponseWriter, r *http.Request, c context.Context, peername string, metadata map[string]string, handlers []OutsideHandler) *Context {
	ctx, ok := s.ctxpool.Get().(*Context)
	if !ok {
		ctx = &Context{
			Context:  c,
			w:        w,
			r:        r,
			peername: peername,
			metadata: metadata,
			handlers: handlers,
			status:   0,
			e:        nil,
		}
		if metadata == nil {
			ctx.metadata = make(map[string]string)
		}
		return ctx
	}
	ctx.Context = c
	ctx.w = w
	ctx.r = r
	ctx.peername = peername
	if metadata != nil {
		ctx.metadata = metadata
	}
	ctx.handlers = handlers
	ctx.status = 0
	ctx.e = nil
	return ctx
}

func (s *WebServer) putContext(ctx *Context) {
	for k := range ctx.metadata {
		delete(ctx.metadata, k)
	}
	ctx.r = nil
	ctx.w = nil
	ctx.body = nil
	ctx.bodyerr = nil
	s.ctxpool.Put(ctx)
}

type Context struct {
	context.Context
	w        http.ResponseWriter
	r        *http.Request
	peername string
	metadata map[string]string
	handlers []OutsideHandler
	status   int8
	body     []byte
	bodyerr  error
	e        *cerror.Error
}

func (c *Context) run() {
	for _, handler := range c.handlers {
		handler(c)
		if c.status != 0 {
			break
		}
	}
}

// has race
func (c *Context) Abort(e error) {
	c.status = -1
	c.e = cerror.ConvertStdError(e)
	if c.e != nil {
		c.w.Header().Set("Content-Type", "application/json")
		if c.e.Httpcode < 400 || c.e.Httpcode > 999 {
			panic("[web.Context.Abort] httpcode must in [400,999]")
		}
		c.w.WriteHeader(int(c.e.Httpcode))
		c.w.Write(common.Str2byte(c.e.Error()))
	}
}

// has race
func (c *Context) Write(contenttype string, msg []byte) {
	c.status = 1
	c.w.Header().Set("Content-Type", contenttype)
	c.w.WriteHeader(http.StatusOK)
	c.w.Write(msg)
}

func (c *Context) WriteString(contenttype, msg string) {
	c.Write(contenttype, common.Str2byte(msg))
}

func (c *Context) Redirect(code int, url string) {
	c.status = 1
	if code != 301 && code != 302 && code != 307 && code != 308 {
		panic("[web.Context.Direct] httpcode must be 301/302/307/308")
	}
	http.Redirect(c.w, c.r, url, code)
}

func (c *Context) SetHeader(k, v string) {
	c.w.Header().Set(k, v)
}
func (c *Context) AddHeader(k, v string) {
	c.w.Header().Add(k, v)
}
func (c *Context) GetRequest() *http.Request {
	return c.r
}
func (c *Context) GetResponse() http.ResponseWriter {
	return c.w
}
func (c *Context) GetHost() string {
	return c.r.URL.Host
}
func (c *Context) GetPath() string {
	return c.r.URL.Path
}
func (c *Context) GetMethod() string {
	return c.r.Method
}
func (c *Context) GetBasicAuth() (string, string, bool) {
	return c.r.BasicAuth()
}
func (c *Context) GetMetadata() map[string]string {
	return c.metadata
}
func (c *Context) GetHeaders() http.Header {
	return c.r.Header
}
func (c *Context) GetHeader(key string) string {
	return c.r.Header.Get(key)
}
func (c *Context) GetPeerName() string {
	return c.peername
}

// get the direct peer's addr(maybe a proxy)
func (c *Context) GetRemoteAddr() string {
	return c.r.RemoteAddr
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *Context) GetClientIp() string {
	return c.metadata["Client-IP"]
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
func (c *Context) GetUserAgent() string {
	return c.r.Header.Get("User-Agent")
}
func (c *Context) GetReferer() string {
	return c.r.Header.Get("Referer")
}
func (c *Context) GetContentType() string {
	return c.r.Header.Get("Content-Type")
}
func (c *Context) GetContentLanguage() string {
	return c.r.Header.Get("Content-Language")
}
func (c *Context) GetContentLength() int64 {
	return c.r.ContentLength
}
func (c *Context) GetAcceptType() string {
	return c.r.Header.Get("Accept")
}
func (c *Context) GetAcceptEncoding() string {
	return c.r.Header.Get("Accept-Encoding")
}
func (c *Context) GetAcceptLanguage() string {
	return c.r.Header.Get("Accept-Language")
}
func (c *Context) GetCookies() []*http.Cookie {
	return c.r.Cookies()
}
func (c *Context) GetCookie(key string) *http.Cookie {
	result, e := c.r.Cookie(key)
	if e == http.ErrNoCookie {
		return nil
	}
	return result
}
func (c *Context) ParseForm() error {
	if e := c.r.ParseForm(); e != nil {
		return e
	}
	if e := c.r.ParseMultipartForm(32 << 20); e != nil && e != http.ErrNotMultipart {
		return e
	}
	return nil
}

// must call ParseForm before this
func (c *Context) GetForm(key string) string {
	if len(c.r.Form) == 0 {
		return ""
	}
	return c.r.Form.Get(key)
}

// must call ParseForm before this
func (c *Context) GetForms(key string) []string {
	if len(c.r.Form) == 0 {
		return nil
	}
	values := c.r.Form[key]
	if len(values) == 0 {
		return nil
	}
	return values
}
func (c *Context) GetBody() ([]byte, error) {
	if c.body != nil || c.bodyerr != nil {
		return c.body, c.bodyerr
	}
	c.body, c.bodyerr = io.ReadAll(c.r.Body)
	return c.body, c.bodyerr
}
