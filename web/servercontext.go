package web

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/util/common"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ServerContext struct {
	context.Context
	w         http.ResponseWriter
	r         *http.Request
	sse       bool
	peerip    string
	responsed atomic.Bool
	closed    atomic.Bool
	e         *cerror.Error
	//w shouldn't be used after ServerHTTP returned,it will panic if the w's Flush() called after ServerHTTP returned
	//we run handler in goroutine to make the timeout work,this is async
	//we need to gurantee the w's Write and Flush not be called after ServeHTTP returned,the atomic can't gurantee this
	//the atomic is used for no timeout mode,when the handler is called in sync mode
	lker *sync.Mutex
}

func (c *ServerContext) Web() {
	//this is a placeholder for NoStreamServerContext interface
}

// ----------------------------------------------- for response------------------------------------------------------

func (c *ServerContext) Redirect(code int, url string) {
	if c.responsed.Swap(true) || c.closed.Swap(true) {
		return
	}
	if code != 301 && code != 302 && code != 303 && code != 307 && code != 308 {
		panic("[web.ServerContext.Redirect] httpcode must be 301/302/303/307/308")
	}
	http.Redirect(c.w, c.r, url, code)
}

func (c *ServerContext) Abort(e error) {
	if c.lker != nil {
		c.lker.Lock()
		defer c.lker.Unlock()
	}
	if c.closed.Swap(true) {
		//when the DeadlineExceeded happened
		//http's handler will return before the user's handler and the closed will be setted to true
		//we should always record the user's error for the trace system
		if ee := cerror.Convert(e); ee != nil {
			c.e = ee
		}
		return
	}
	responsed := c.responsed.Swap(true)
	httpcode := 0
	if ee := cerror.Convert(e); ee != nil {
		if http.StatusText(int(ee.GetHttpcode())) == "" || ee.GetHttpcode() < 400 {
			c.e = cerror.ErrPanic
			httpcode = int(ee.GetHttpcode())
		} else {
			c.e = ee
		}
	}
	if c.e != nil {
		if !responsed {
			//not responsed before,return in normal http mode
			c.w.Header().Set("Content-Type", "application/json")
			c.w.WriteHeader(int(c.e.GetHttpcode()))
			c.w.Write(common.STB(c.e.Json()))
			c.w.(http.Flusher).Flush()
		} else if c.sse {
			//already responsed before in ServerSentEvent mode
			//return error in error event
			d := c.e.Json()
			msg := make([]byte, 0, 21+len(d))
			msg = append(msg, "event: error\ndata: "...)
			msg = append(msg, d...)
			msg = append(msg, "\n\n"...)
			c.w.Write(msg)
			c.w.(http.Flusher).Flush()
		} else {
			//already responsed before in normal http mode
			//we don't known how to handle this
		}
	}
	if httpcode != 0 {
		panic("[web.ServerContext] unknown http code: " + strconv.Itoa(httpcode))
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

// Write all the data or return error
func (c *ServerContext) Write(msg []byte) (int, error) {
	if c.lker != nil {
		c.lker.Lock()
		defer c.lker.Unlock()
	}
	switch c.Context.Err() {
	case context.DeadlineExceeded:
		return 0, cerror.ErrDeadlineExceeded
	case context.Canceled:
		return 0, cerror.ErrClosed
	default:
	}
	if c.closed.Load() {
		return 0, cerror.ErrClosed
	}
	c.responsed.Store(true)
	n, e := c.w.Write(msg)
	if e != nil {
		return n, cerror.ErrClosed
	}
	return n, nil
}
func (c *ServerContext) Flush() {
	if c.lker != nil {
		c.lker.Lock()
		defer c.lker.Unlock()
	}
	if c.closed.Load() {
		return
	}
	c.w.(http.Flusher).Flush()
}
func (c *ServerContext) Responsed() bool {
	return c.responsed.Load()
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

// ----------------------------------------------- for protobuf ------------------------------------------------------

// ----------------------------------------------- no stream context ---------------------------------------------
type NoStreamServerContext interface {
	Web()
	//request
	GetRequest() *http.Request
	GetRemoteAddr() string
	GetRealPeerIp() string
	GetClientIp() string

	//response
	Redirect(code int, url string)
	SetResponseHeader(k, v string)
	AddResponseHeader(k, v string)
}

// --------------------------------------------- server stream context ------------------------------------------
func NewServerStreamServerContext[resptype any](ctx *ServerContext) *ServerStreamServerContext[resptype] {
	cctx := &ServerStreamServerContext[resptype]{Context: ctx.Context, sctx: ctx}
	ctx.responsed.Store(true)
	ctx.SetResponseHeader("Content-Type", "text/event-stream")
	ctx.SetResponseHeader("Cache-Control", "no-cache")
	ctx.SetResponseHeader("Connection", "keep-alive")
	ctx.Flush()
	return cctx
}

type ServerStreamServerContext[resptype any] struct {
	context.Context
	sctx *ServerContext
}

// for Server Sent Events without retry and event support(use the default event:message)
// return cerror.ErrClosed means connection closed
// return cerror.ErrDeadlineExceeded means timeout
func (c *ServerStreamServerContext[resptype]) Send(id string, resp *resptype) error {
	var tmp any = resp
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-web's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.sctx.GetRequest().URL.Path+"] response struct's type is not proto's message")
		return cerror.ErrSystem
	}
	d, _ := (protojson.MarshalOptions{UseProtoNames: true, UseEnumNumbers: true}).Marshal(tmptmp)
	var msg []byte
	if len(id) > 0 {
		msg = make([]byte, 0, len(id)+5+len(d)+8)
		msg = append(msg, "id: "...)
		msg = append(msg, id...)
		msg = append(msg, '\n')
	} else {
		msg = make([]byte, 0, len(d)+8)
	}
	msg = append(msg, "data: "...)
	msg = append(msg, d...)
	msg = append(msg, "\n\n"...)
	if _, e := c.sctx.Write(msg); e != nil {
		slog.ErrorContext(c.Context, "["+c.sctx.GetRequest().URL.Path+"] send response failed", slog.String("error", e.Error()))
		return e
	}
	c.sctx.Flush()
	return nil
}

// can get the Last-Event-ID from header
func (c *ServerStreamServerContext[resptype]) GetRequest() *http.Request {
	return c.sctx.GetRequest()
}
func (c *ServerStreamServerContext[resptype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}
func (c *ServerStreamServerContext[resptype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}
func (c *ServerStreamServerContext[resptype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}
