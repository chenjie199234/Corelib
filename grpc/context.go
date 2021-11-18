package grpc

import (
	"context"

	cerror "github.com/chenjie199234/Corelib/error"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (s *GrpcServer) getcontext(c context.Context, path string, peername string, peeraddr string, metadata map[string]string, handlers []OutsideHandler, d func(interface{}) error) *Context {
	ctx, ok := s.ctxpool.Get().(*Context)
	if !ok {
		return &Context{
			Context:    c,
			decodefunc: d,
			handlers:   handlers,
			path:       path,
			peername:   peername,
			peeraddr:   peeraddr,
			metadata:   metadata,
			resp:       nil,
			e:          nil,
			status:     0,
		}
	}
	ctx.Context = c
	ctx.decodefunc = d
	ctx.handlers = handlers
	ctx.path = path
	ctx.peername = peername
	ctx.peeraddr = peeraddr
	ctx.metadata = metadata
	ctx.resp = nil
	ctx.e = nil
	ctx.status = 0
	return ctx
}
func (s *GrpcServer) putcontext(ctx *Context) {
	s.ctxpool.Put(ctx)
}

type Context struct {
	context.Context
	decodefunc func(interface{}) error
	handlers   []OutsideHandler
	path       string
	peername   string
	peeraddr   string
	metadata   map[string]string
	resp       interface{}
	e          *cerror.Error
	status     int8
}

func (c *Context) run() {
	for _, handler := range c.handlers {
		handler(c)
		if c.status != 0 {
			break
		}
	}
}

//has race
func (c *Context) Abort(e error) {
	c.status = -1
	c.e = cerror.ConvertStdError(e)
	if c.e != nil && (c.e.Httpcode < 400 || c.e.Httpcode > 999) {
		panic("[grpc.Context.Abort] httpcode must in [400,999]")
	}
}

//has race
func (c *Context) Write(resp interface{}) {
	c.status = 1
	c.resp = resp
}
func (c *Context) DecodeReq(req protoreflect.ProtoMessage) error {
	return c.decodefunc(req)
}
func (c *Context) GetPath() string {
	return c.path
}
func (c *Context) GetPeerName() string {
	return c.peername
}
func (c *Context) GetPeerAddr() string {
	return c.peeraddr
}
func (c *Context) GetMetadata() map[string]string {
	return c.metadata
}
