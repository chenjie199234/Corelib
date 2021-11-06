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
		}
	}
	result.Context = ctx
	result.peer = p
	result.msg = msg
	result.handlers = handlers
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
	next     int8
}

func (c *Context) Next() {
	if c.next < 0 {
		return
	}
	c.next++
	for c.next < int8(len(c.handlers)) {
		c.handlers[c.next](c)
		if c.next < 0 {
			break
		}
		c.next++
	}
}

func (c *Context) Abort(e error) {
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = nil
	if e == context.DeadlineExceeded {
		c.msg.Error = cerror.ErrDeadlineExceeded
	} else if e == context.Canceled {
		c.msg.Error = cerror.ErrCanceled
	} else {
		c.msg.Error = cerror.ConvertStdError(e)
	}
	c.msg.Metadata = nil
	c.msg.Tracedata = nil
	c.next = -1
}

func (c *Context) Write(resp []byte) {
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = resp
	c.msg.Error = nil
	c.msg.Metadata = nil
	c.msg.Tracedata = nil
	c.next = -1
}

func (c *Context) WriteString(resp string) {
	c.Write(common.Str2byte(resp))
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
