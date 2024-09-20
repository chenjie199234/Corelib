package cgrpc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

type ServerContext struct {
	context.Context
	decodefunc func(any) error
	stream     grpc.ServerStream
	path       string
	peerip     string
	resp       any
	e          *cerror.Error
	finish     int32
}

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
	if httpcode != 0 {
		panic("[cgrpc.Context.Abort] unknown http code: " + strconv.Itoa(httpcode))
	}
}

// Warning!this function is only used for generated code,don't use it in any other place
func (c *ServerContext) Read(req any) error {
	if c.stream != nil {
		return c.stream.RecvMsg(req)
	}
	return c.decodefunc(req)
}

// Warning!this function is only used for generated code,don't use it in any other place
func (c *ServerContext) Write(resp any) error {
	if c.stream != nil {
		return c.stream.SendMsg(resp)
	}
	c.resp = resp
	return nil
}
func (c *ServerContext) GetMethod() string {
	return "GRPC"
}
func (c *ServerContext) GetPath() string {
	return c.path
}

// get the direct peer's addr(maybe a proxy)
func (c *ServerContext) GetRemoteAddr() string {
	conninfo := c.Context.Value(serverconnkey{}).(*stats.ConnTagInfo)
	return conninfo.RemoteAddr.String()
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

// ----------------------------------------------- no stream context ---------------------------------------------
func NewNoStreamServerContext(ctx *ServerContext) *NoStreamServerContext {
	if ctx.decodefunc == nil {
		return nil
	}
	return &NoStreamServerContext{Context: ctx.Context, sctx: ctx}
}

type NoStreamServerContext struct {
	context.Context
	sctx *ServerContext
}

func (c *NoStreamServerContext) GetPath() string {
	return c.sctx.GetPath()
}

// get the direct peer's addr(maybe a proxy)
func (c *NoStreamServerContext) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}

// get the real peer's ip which will not be confused by proxy
func (c *NoStreamServerContext) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *NoStreamServerContext) GetClientIp() string {
	return c.sctx.GetClientIp()
}

// ----------------------------------------------- client stream context ---------------------------------------------
func NewClientStreamServerContext[reqtype any](ctx *ServerContext, validatereq bool) *ClientStreamServerContext[reqtype] {
	if ctx.stream == nil {
		return nil
	}
	return &ClientStreamServerContext[reqtype]{
		validatereq: validatereq,
		Context:     ctx.Context,
		sctx:        ctx,
	}
}

type ClientStreamServerContext[reqtype any] struct {
	validatereq bool
	context.Context
	sctx *ServerContext
}

func (c *ClientStreamServerContext[reqtype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	if e := c.sctx.stream.RecvMsg(req); e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.sctx.path+"] read request failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	if c.validatereq {
		if v, ok := req.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.sctx.path+"] request validate failed", slog.String("error", errstr))
				return nil, cerror.ErrReq
			}
		}
	}
	return req.(*reqtype), nil
}
func (c *ClientStreamServerContext[reqtype]) GetPath() string {
	return c.sctx.GetPath()
}

// get the direct peer's addr(maybe a proxy)
func (c *ClientStreamServerContext[reqtype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}

// get the real peer's ip which will not be confused by proxy
func (c *ClientStreamServerContext[reqtype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *ClientStreamServerContext[reqtype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}

// ----------------------------------------------- server stream context ---------------------------------------------
func NewServerStreamServerContext[resptype any](ctx *ServerContext) *ServerStreamServerContext[resptype] {
	if ctx.stream == nil {
		return nil
	}
	return &ServerStreamServerContext[resptype]{
		Context: ctx.Context,
		sctx:    ctx,
	}
}

type ServerStreamServerContext[resptype any] struct {
	context.Context
	sctx *ServerContext
}

// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *ServerStreamServerContext[resptype]) Send(resp *resptype) error {
	e := c.sctx.stream.SendMsg(resp)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.sctx.path+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ServerStreamServerContext[resptype]) GetPath() string {
	return c.sctx.GetPath()
}

// get the direct peer's addr(maybe a proxy)
func (c *ServerStreamServerContext[resptype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}

// get the real peer's ip which will not be confused by proxy
func (c *ServerStreamServerContext[resptype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *ServerStreamServerContext[resptype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}

// ----------------------------------------------- all stream context ---------------------------------------------
func NewAllStreamServerContext[reqtype, resptype any](ctx *ServerContext, validatereq bool) *AllStreamServerContext[reqtype, resptype] {
	if ctx.stream == nil {
		return nil
	}
	return &AllStreamServerContext[reqtype, resptype]{
		validatereq: validatereq,
		Context:     ctx.Context,
		sctx:        ctx,
	}
}

type AllStreamServerContext[reqtype, resptype any] struct {
	validatereq bool
	context.Context
	sctx *ServerContext
}

func (c *AllStreamServerContext[reqtype, resptype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	if e := c.sctx.stream.RecvMsg(req); e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.sctx.path+"] read request failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	if c.validatereq {
		if v, ok := req.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.sctx.path+"] request validate failed", slog.String("error", errstr))
				return nil, cerror.ErrReq
			}
		}
	}
	return req.(*reqtype), nil
}

// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *AllStreamServerContext[reqtype, resptype]) Send(resp *resptype) error {
	e := c.sctx.stream.SendMsg(resp)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.sctx.path+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamServerContext[reqtype, resptype]) GetPath() string {
	return c.sctx.GetPath()
}

// get the direct peer's addr(maybe a proxy)
func (c *AllStreamServerContext[reqtype, resptype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}

// get the real peer's ip which will not be confused by proxy
func (c *AllStreamServerContext[reqtype, resptype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *AllStreamServerContext[reqtype, resptype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}
