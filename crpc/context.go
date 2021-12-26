package crpc

import (
	"context"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
)

func (s *CrpcServer) getContext(ctx context.Context, p *stream.Peer, msg *Msg, handlers []OutsideHandler) *Context {
	result, ok := s.ctxpool.Get().(*Context)
	if !ok {
		return &Context{
			Context:  ctx,
			peer:     p,
			msg:      msg,
			handlers: handlers,
			status:   0,
		}
	}
	result.Context = ctx
	result.peer = p
	result.msg = msg
	result.handlers = handlers
	result.status = 0
	return result
}

func (s *CrpcServer) putContext(ctx *Context) {
	s.ctxpool.Put(ctx)
}

type Context struct {
	context.Context
	msg      *Msg
	peer     *stream.Peer
	handlers []OutsideHandler
	status   int8
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
	c.msg.Error = cerror.ConvertStdError(e)
	if c.msg.Error != nil && (c.msg.Error.Httpcode < 400 || c.msg.Error.Httpcode == 888 || c.msg.Error.Httpcode > 999) {
		panic("[crpc.Context.Abort] httpcode must in [400,888) or (888,999]")
	}
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = nil
	c.msg.Metadata = nil
	c.msg.Tracedata = nil
	c.status = -1
}

//has race
func (c *Context) Write(resp []byte) {
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = resp
	c.msg.Error = nil
	c.msg.Metadata = nil
	c.msg.Tracedata = nil
	c.status = 1
}

func (c *Context) WriteString(resp string) {
	c.Write(common.Str2byte(resp))
}

func (c *Context) GetMethod() string {
	return "CRPC"
}
func (c *Context) GetPath() string {
	return c.msg.Path
}
func (c *Context) GetBody() []byte {
	return c.msg.Body
}
func (c *Context) GetPeerName() string {
	return c.peer.GetPeerName()
}
func (c *Context) GetPeerAddr() string {
	return c.peer.GetRemoteAddr()
}
func (c *Context) GetMetadata() map[string]string {
	return c.msg.Metadata
}
