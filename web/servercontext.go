package web

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/util/common"
)

type ServerContext struct {
	context.Context
	w       http.ResponseWriter
	r       *http.Request
	peerip  string
	finish  int32
	body    []byte
	bodyerr error
	e       *cerror.Error
}

func (c *ServerContext) Web() {
	//this is a placeholder for NoStreamServerContext interface
}

// ----------------------------------------------- for response------------------------------------------------------

func (c *ServerContext) Redirect(code int, url string) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
	if code != 301 && code != 302 && code != 303 && code != 307 && code != 308 {
		panic("[web.ServerContext.Redirect] httpcode must be 301/302/303/307/308")
	}
	http.Redirect(c.w, c.r, url, code)
}

// if e is not nil,it will be converted to cerror.Error and will set the response code to 4xx or 5xx,see the Httpcode in cerror.Error
func (c *ServerContext) Abort(e error) {
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
		c.w.Header().Set("Content-Type", "application/json")
		c.w.WriteHeader(int(c.e.Httpcode))
		c.w.Write(common.STB(c.e.Json()))
	}
	if httpcode != 0 {
		panic("[web.ServerContext.Abort] unknown http code: " + strconv.Itoa(httpcode))
	}
}

// after Write,this will be useless
func (c *ServerContext) SetResponseHeader(k, v string) {
	c.w.Header().Set(k, v)
}

// after Write,this will be useless
func (c *ServerContext) AddResponseHeader(k, v string) {
	c.w.Header().Add(k, v)
}

func (c *ServerContext) Write(msg []byte) (int, error) {
	if atomic.LoadInt32(&c.finish) != 0 {
		panic("[web.ServerContext] write on already finished Context")
	}
	return c.w.Write(msg)
}

func (c *ServerContext) Flush() {
	c.w.(http.Flusher).Flush()
}

// ----------------------------------------------- for request------------------------------------------------------

func (c *ServerContext) GetRequest() *http.Request {
	return c.r
}

// get the direct peer's addr(maybe a proxy)
func (c *ServerContext) GetRemoteAddr() string {
	return c.r.RemoteAddr
}

// get the real peer's ip which will not be confused by proxy
func (c *ServerContext) GetRealPeerIp() string {
	return c.peerip
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *ServerContext) GetClientIp() string {
	md := metadata.GetMetadata(c.Context)
	return md["Client-IP"]
}

func (c *ServerContext) GetBody() ([]byte, error) {
	if c.body != nil || c.bodyerr != nil {
		return c.body, c.bodyerr
	}
	b := make([]byte, 0, c.r.ContentLength)
	for {
		n, e := c.r.Body.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if e != nil {
			if e != io.EOF {
				c.bodyerr = e
			}
			break
		}
		if len(b) == cap(b) {
			//aready read at least Content-Length body,still not EOF
			c.bodyerr = cerror.ErrReq
			break
		}
	}
	if c.bodyerr == nil {
		c.body = b
	}
	return c.body, c.bodyerr
}

// ----------------------------------------------- for protobuf ------------------------------------------------------

type NoStreamServerContext interface {
	Web()
	//request
	GetRequest() *http.Request
	GetRemoteAddr() string
	GetRealPeerIp() string
	GetClientIp() string
	GetBody() ([]byte, error)

	//response
	Redirect(code int, url string)
	Abort(error)
	SetResponseHeader(k, v string)
	AddResponseHeader(k, v string)
}

type ServerStreamServerContext struct {
}
