package crpc

import (
	"context"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/stream"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ServerContext struct {
	context.Context
	cancel context.CancelFunc
	rw     *rw
	peer   *stream.Peer
	peerip string
	finish int32
	e      *cerror.Error
}

func (c *ServerContext) Abort(e error) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
	ee := cerror.Convert(e)
	if ee == nil {
		c.rw.closesend()
	} else if ee.Httpcode < 400 || ee.Httpcode > 999 {
		panic("[crpc.Context.Abort] http code must in [400,999)")
	} else {
		c.e = ee
		c.rw.send(&MsgBody{Error: c.e})
	}
}

func (c *ServerContext) Send(resp []byte) error {
	return c.rw.send(&MsgBody{Body: resp})
}
func (c *ServerContext) StopSend() {
	c.rw.closesend()
}

func (c *ServerContext) Read() ([]byte, error) {
	body, e := c.rw.read()
	return body, e
}
func (c *ServerContext) StopRead() {
	c.rw.closeread()
}

func (c *ServerContext) GetMethod() string {
	return "CRPC"
}
func (c *ServerContext) GetPath() string {
	return c.rw.path
}

// get the direct peer's addr(maybe a proxy)
func (c *ServerContext) GetRemoteAddr() string {
	return c.peer.GetRemoteAddr()
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

// ----------------------------------------------- client stream context ---------------------------------------------

func NewClientStreamServerContext[reqtype protoreflect.ProtoMessage](ctx *ServerContext, newreq func() reqtype) *ClientStreamServerContext[reqtype] {
	return &ClientStreamServerContext[reqtype]{Context: ctx.Context, sctx: ctx, newreq: newreq}
}

type ClientStreamServerContext[reqtype protoreflect.ProtoMessage] struct {
	context.Context
	sctx   *ServerContext
	newreq func() reqtype
}

func (c *ClientStreamServerContext[reqtype]) Read() (reqtype, error) {
	data, e := c.sctx.Read()
	if e != nil {
		var empty reqtype
		return empty, e
	}
	req := c.newreq()
	if e := proto.Unmarshal(data, req); e != nil {
		var empty reqtype
		return empty, e
	}
	return req, nil
}
func (c *ClientStreamServerContext[reqtype]) StopRead() {
	c.StopRead()
}
func (c *ClientStreamServerContext[reqtype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}
func (c *ClientStreamServerContext[reqtype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}
func (c *ClientStreamServerContext[reqtype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}

// ----------------------------------------------- server stream context ---------------------------------------------

func NewServerStreamServerContext[resptype protoreflect.ProtoMessage](ctx *ServerContext) *ServerStreamServerContext[resptype] {
	return &ServerStreamServerContext[resptype]{Context: ctx.Context, sctx: ctx}
}

type ServerStreamServerContext[resptype protoreflect.ProtoMessage] struct {
	context.Context
	sctx *ServerContext
}

func (c *ServerStreamServerContext[resptype]) Send(resp resptype) error {
	d, e := proto.Marshal(resp)
	if e != nil {
		return e
	}
	return c.sctx.Send(d)
}
func (c *ServerStreamServerContext[resptype]) StopSend() {
	c.sctx.StopSend()
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

// ----------------------------------------------- all stream context ---------------------------------------------

func NewAllStreamServerContext[reqtype, resptype protoreflect.ProtoMessage](ctx *ServerContext, newreq func() reqtype) *AllStreamServerContext[reqtype, resptype] {
	return &AllStreamServerContext[reqtype, resptype]{sctx: ctx, Context: ctx.Context, newreq: newreq}
}

type AllStreamServerContext[reqtype, resptype protoreflect.ProtoMessage] struct {
	context.Context
	sctx   *ServerContext
	newreq func() reqtype
}

func (c *AllStreamServerContext[reqtype, resptype]) Read() (reqtype, error) {
	data, e := c.sctx.Read()
	if e != nil {
		var empty reqtype
		return empty, e
	}
	req := c.newreq()
	if e := proto.Unmarshal(data, req); e != nil {
		var empty reqtype
		return empty, e
	}
	return req, nil
}
func (c *AllStreamServerContext[reqtype, resptype]) StopRead() {
	c.sctx.StopRead()
}
func (c *AllStreamServerContext[reqtype, resptype]) Send(resp resptype) error {
	d, e := proto.Marshal(resp)
	if e != nil {
		return e
	}
	return c.sctx.Send(d)
}
func (c *AllStreamServerContext[reqtype, resptype]) StopSend() {
	c.sctx.StopSend()
}
func (c *AllStreamServerContext[reqtype, resptype]) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}
func (c *AllStreamServerContext[reqtype, resptype]) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}
func (c *AllStreamServerContext[reqtype, resptype]) GetClientIp() string {
	return c.sctx.GetClientIp()
}
