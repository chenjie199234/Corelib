package web

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/util/common"
)

type Context struct {
	context.Context
	w       http.ResponseWriter
	r       *http.Request
	peerip  string
	finish  int32
	body    []byte
	bodyerr error
	e       *cerror.Error
}

func (c *Context) Abort(e error) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
	httpcode := 0
	if ee := cerror.Convert(e); ee != nil {
		if http.StatusText(int(ee.Httpcode)) == "" || ee.Httpcode < 400 {
			c.e = cerror.ErrPanic
			httpcode = int(ee.Httpcode)
		} else {
			c.e = ee
		}
	}
	if c.e != nil {
		lastcpu, _, _ := cotel.GetCPU()
		c.w.Header().Set("Cpu-Usage", strconv.FormatFloat(lastcpu, 'g', 10, 64))
		c.w.Header().Set("Content-Type", "application/json")
		c.w.WriteHeader(int(c.e.Httpcode))
		c.w.Write(common.STB(c.e.Json()))
	}
	if httpcode != 0 {
		panic("[web.Context.Abort] unknown http code: " + strconv.Itoa(httpcode))
	}
}

func (c *Context) Write(contenttype string, msg []byte) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
	lastcpu, _, _ := cotel.GetCPU()
	c.w.Header().Set("Cpu-Usage", strconv.FormatFloat(lastcpu, 'g', 10, 64))
	c.w.Header().Set("Content-Type", contenttype)
	c.w.WriteHeader(http.StatusOK)
	c.w.Write(msg)
}

func (c *Context) Redirect(code int, url string) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
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
func (c *Context) GetHeaders() http.Header {
	return c.r.Header
}
func (c *Context) GetHeader(key string) string {
	return c.r.Header.Get(key)
}

// get the direct peer's addr(maybe a proxy)
func (c *Context) GetRemoteAddr() string {
	return c.r.RemoteAddr
}

// get the real peer's ip which will not be confused by proxy
func (c *Context) GetRealPeerIp() string {
	return c.peerip
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *Context) GetClientIp() string {
	md := metadata.GetMetadata(c.Context)
	return md["Client-IP"]
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
func (c *Context) GetTransferEncoding() string {
	return c.r.Header.Get("Transfer-Encoding")
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
	b := bpool.Get(int(c.GetContentLength()))
	for {
		n, e := c.r.Body.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if e != nil && e == io.EOF {
			break
		}
		if e != nil {
			c.bodyerr = e
			bpool.Put(&b)
			break
		}
		if len(b) == cap(b) {
			//aready read at least Content-Length body,still not EOF
			c.bodyerr = cerror.ErrReq
			bpool.Put(&b)
			break
		}
	}
	if c.bodyerr == nil {
		c.body = b
	}
	return c.body, c.bodyerr
}
