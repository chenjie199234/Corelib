package cgrpc

import (
	"context"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (s *CGrpcServer) getcontext(c context.Context, path string, peername string, remoteaddr string, handlers []OutsideHandler, d func(interface{}) error) *Context {
	ctx, ok := s.ctxpool.Get().(*Context)
	if !ok {
		ctx = &Context{
			Context:    c,
			decodefunc: d,
			handlers:   handlers,
			path:       path,
			peername:   peername,
			remoteaddr: remoteaddr,
			resp:       nil,
			e:          nil,
			finish:     0,
		}
		return ctx
	}
	ctx.Context = c
	ctx.decodefunc = d
	ctx.handlers = handlers
	ctx.path = path
	ctx.peername = peername
	ctx.remoteaddr = remoteaddr
	ctx.resp = nil
	ctx.e = nil
	ctx.finish = 0
	return ctx
}
func (s *CGrpcServer) putcontext(ctx *Context) {
	s.ctxpool.Put(ctx)
}

type Context struct {
	context.Context
	decodefunc func(interface{}) error
	handlers   []OutsideHandler
	path       string
	peername   string
	remoteaddr string
	resp       interface{}
	e          *cerror.Error
	finish     int32
}

func (c *Context) run() {
	for _, handler := range c.handlers {
		handler(c)
		if c.finish != 0 {
			break
		}
	}
}

// has race
func (c *Context) Abort(e error) {
	if !atomic.CompareAndSwapInt32(&c.finish, 0, -1) {
		return
	}
	c.e = cerror.ConvertStdError(e)
	if c.e != nil && (c.e.Httpcode < 400 || c.e.Httpcode > 999) {
		panic("[cgrpc.Context.Abort] httpcode must in [400,999]")
	}
}

// has race
func (c *Context) Write(resp interface{}) {
	if !atomic.CompareAndSwapInt32(&c.finish, 0, -1) {
		return
	}
	c.resp = resp
}
func (c *Context) DecodeReq(req protoreflect.ProtoMessage) error {
	return c.decodefunc(req)
}
func (c *Context) GetMethod() string {
	return "GRPC"
}
func (c *Context) GetPath() string {
	return c.path
}
func (c *Context) GetPeerName() string {
	return c.peername
}

// get the direct peer's addr(maybe a proxy)
func (c *Context) GetRemoteAddr() string {
	return c.remoteaddr
}

// this function try to return the first caller's ip(mostly time it will be the user's ip)
// if can't get the first caller's ip,try to return the real peer's ip which will not be confused by proxy
// if failed,the direct peer's ip will be returned(maybe a proxy)
func (c *Context) GetClientIp() string {
	md := metadata.GetMetadata(c.Context)
	return md["Client-IP"]
}
