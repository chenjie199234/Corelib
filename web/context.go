package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/metadata"
	"github.com/julienschmidt/httprouter"
)

type Context struct {
	context.Context
	s        *WebServer
	w        http.ResponseWriter
	r        *http.Request
	handlers []OutsideHandler
	next     int8
}

func (this *Context) Next() {
	if this.next < 0 {
		return
	}
	this.next++
	for this.next < int8(len(this.handlers)) {
		this.handlers[this.next](this)
		if this.next < 0 {
			break
		}
		this.next++
	}
}

func (this *Context) Abort(code int, msg []byte) error {
	this.w.WriteHeader(code)
	if len(msg) > 0 {
		this.w.Write(msg)
	}
	this.next = -1
	return nil
}

func (this *Context) AbortString(code int, msg string) error {
	return this.Abort(code, common.Str2byte(msg))
}

func (this *Context) Write(code int, msg []byte) error {
	this.w.WriteHeader(code)
	if len(msg) > 0 {
		this.w.Write(msg)
	}
	this.next = -1
	return nil
}

func (this *Context) WriteString(code int, msg string) error {
	return this.Write(code, common.Str2byte(msg))
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
	return metadata.GetAllMetadata(this.r.Context())
}
func (this *Context) GetHeaders() http.Header {
	return this.r.Header
}
func (this *Context) GetHeader(key string) string {
	//return "" means not exists
	return this.r.Header.Get(key)
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
func (this *Context) GetSourceServer() string {
	return this.r.Header.Get("SourceServer")
}
func (this *Context) GetClientIp() string {
	return getclientip(this.r)
}
func (this *Context) GetRemoteAddr() string {
	return this.r.RemoteAddr
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

//Cookies
func (this *Context) GetCookies() []*http.Cookie {
	return this.r.Cookies()
}

//return nil means not exists
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
	//return "" means not exists
	return this.r.Form.Get(key)
}
func (this *Context) GetBody() ([]byte, error) {
	//return nil means not exists
	return io.ReadAll(this.r.Body)
}

//param is the value in dynamic url,see httprouter's dynamic path
//https://github.com/julienschmidt/httprouter
func (this *Context) GetParams() httprouter.Params {
	//return nil means not exists
	return httprouter.ParamsFromContext(this.Context)
}

//param is the value in dynamic url,see httprouter's dynamic path
//https://github.com/julienschmidt/httprouter
func (this *Context) GetParam(key string) string {
	//return "" means not exists
	return httprouter.ParamsFromContext(this.Context).ByName(key)
}
