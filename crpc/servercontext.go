package crpc

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/pbex"
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

func (c *ServerContext) Recv() ([]byte, error) {
	body, e := c.rw.read()
	return body, e
}
func (c *ServerContext) StopRecv() {
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

// ----------------------------------------------- no stream context ---------------------------------------------

func NewNoStreamServerContext(path string, ctx *ServerContext) *NoStreamServerContext {
	return &NoStreamServerContext{path: path, Context: ctx.Context, sctx: ctx}
}

type NoStreamServerContext struct {
	path string
	context.Context
	sctx *ServerContext
}

func (c *NoStreamServerContext) GetPath() string {
	return c.path
}
func (c *NoStreamServerContext) GetRemoteAddr() string {
	return c.sctx.GetRemoteAddr()
}
func (c *NoStreamServerContext) GetRealPeerIp() string {
	return c.sctx.GetRealPeerIp()
}
func (c *NoStreamServerContext) GetClientIp() string {
	return c.sctx.GetClientIp()
}

// ----------------------------------------------- client stream context ---------------------------------------------

func NewClientStreamServerContext[reqtype any](path string, ctx *ServerContext, validatereq bool) *ClientStreamServerContext[reqtype] {
	return &ClientStreamServerContext[reqtype]{path: path, Context: ctx.Context, sctx: ctx, validatereq: validatereq}
}

type ClientStreamServerContext[reqtype any] struct {
	path        string
	validatereq bool
	context.Context
	sctx *ServerContext
}

func (c *ClientStreamServerContext[reqtype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	m, ok := req.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] request struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, e := c.sctx.Recv()
	if e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] read request failed", slog.String("error", e.Error()))
		return nil, e
	}
	if e := proto.Unmarshal(data, m); e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] request decode failed", slog.String("error", e.Error()))
		return nil, e
	}
	if c.validatereq {
		if v, ok := req.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.path+"] request validate failed", slog.String("error", errstr))
				return nil, cerror.ErrReq
			}
		}
	}
	return req.(*reqtype), nil
}
func (c *ClientStreamServerContext[reqtype]) StopRecv() {
	c.StopRecv()
}
func (c *ClientStreamServerContext[reqtype]) GetPath() string {
	return c.path
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

func NewServerStreamServerContext[resptype any](path string, ctx *ServerContext) *ServerStreamServerContext[resptype] {
	return &ServerStreamServerContext[resptype]{path: path, Context: ctx.Context, sctx: ctx}
}

type ServerStreamServerContext[resptype any] struct {
	path string
	context.Context
	sctx *ServerContext
}

func (c *ServerStreamServerContext[resptype]) Send(resp *resptype) error {
	var tmp any = resp
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] response struct's type is not proto's message")
		return cerror.ErrSystem
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.sctx.Send(d)
	if e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ServerStreamServerContext[resptype]) StopSend() {
	c.sctx.StopSend()
}
func (c *ServerStreamServerContext[resptype]) GetPath() string {
	return c.sctx.GetPath()
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

func NewAllStreamServerContext[reqtype, resptype any](path string, ctx *ServerContext, validatereq bool) *AllStreamServerContext[reqtype, resptype] {
	return &AllStreamServerContext[reqtype, resptype]{path: path, sctx: ctx, Context: ctx.Context, validatereq: validatereq}
}

type AllStreamServerContext[reqtype, resptype any] struct {
	path        string
	validatereq bool
	context.Context
	sctx *ServerContext
}

func (c *AllStreamServerContext[reqtype, resptype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	m, ok := req.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] request struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, e := c.sctx.Recv()
	if e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] read request failed", slog.String("error", e.Error()))
		return nil, e
	}
	if e := proto.Unmarshal(data, m); e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] request decode failed", slog.String("error", e.Error()))
		return nil, e
	}
	if c.validatereq {
		if v, ok := req.(interface{ Validate() string }); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.path+"] request validate failed", slog.String("error", errstr))
				return nil, cerror.ErrReq
			}
		}
	}
	return req.(*reqtype), nil
}
func (c *AllStreamServerContext[reqtype, resptype]) StopRecv() {
	c.sctx.StopRecv()
}
func (c *AllStreamServerContext[reqtype, resptype]) Send(resp *resptype) error {
	var tmp any = resp
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] response struct's type is not proto's message")
		return cerror.ErrSystem
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.sctx.Send(d)
	if e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamServerContext[reqtype, resptype]) StopSend() {
	c.sctx.StopSend()
}
func (c *AllStreamServerContext[reqtype, resptype]) GetPath() string {
	return c.sctx.GetPath()
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
