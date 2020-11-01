package web

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"

	"github.com/chenjie199234/Corelib/common"
	"github.com/julienschmidt/httprouter"
)

type Context struct {
	context.Context
	w http.ResponseWriter
	r *http.Request
	p httprouter.Params
	s bool
}

//Params
func (this *Context) GetParams() httprouter.Params {
	return this.p
}

//return "" means not exists
func (this *Context) GetParam(key string) string {
	return this.p.ByName(key)
}

//Headers
func (this *Context) GetHeaders() http.Header {
	return this.r.Header
}

//return "" means not exists
func (this *Context) GetHeader(key string) string {
	return this.r.Header.Get(key)
}

func (this *Context) GetRemoteAddr() string {
	if ip := this.r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	} else if ip = this.r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	ip, _, _ := net.SplitHostPort(this.r.RemoteAddr)
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
func (this *Context) GetContentLength() int64 {
	return this.r.ContentLength
}
func (this *Context) GetAcceptType() string {
	return this.r.Header.Get("Accept")
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
	result, _ := this.r.Cookie(key)
	return result
}

func (this *Context) GetUsername() *http.Cookie {
	result, _ := this.r.Cookie("Username")
	return result
}
func (this *Context) GetSession() *http.Cookie {
	result, _ := this.r.Cookie("SessionID")
	return result
}

//Form
func (this *Context) GetForms() url.Values {
	if len(this.r.Form) == 0 {
		this.r.ParseForm()
	}
	return this.r.Form
}

//return "" means not exists
func (this *Context) GetForm(key string) string {
	if len(this.r.Form) == 0 {
		this.r.ParseForm()
	}
	return this.r.Form.Get(key)
}

//return nil means not exists
func (this *Context) GetBody() ([]byte, error) {
	if this.r.ContentLength == 0 {
		return nil, nil
	}
	return ioutil.ReadAll(this.r.Body)
}

var ErrStatusCodeNotExist = fmt.Errorf("status code doesn't exist")

func (this *Context) Write(code int, data []byte) error {
	if str := http.StatusText(code); str == "" {
		return ErrStatusCodeNotExist
	}
	this.w.WriteHeader(code)
	this.w.Write(data)
	this.s = false
	return nil
}

func (this *Context) WriteString(code int, data string) error {
	return this.Write(code, common.Str2byte(data))
}
