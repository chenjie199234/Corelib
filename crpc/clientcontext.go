package crpc

import (
	"context"
	"io"
	"log/slog"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallContext struct {
	context.Context
	rw *rw
	s  *ServerForPick
}

// return io.EOF means server stop send
func (c *CallContext) Recv() ([]byte, error) {
	return c.rw.recv()
}
func (c *CallContext) StopRecv() {
	c.rw.closerecv()
}
func (c *CallContext) GetServerAddr() string {
	return c.s.GetServerAddr()
}

type StreamContext struct {
	context.Context
	rw *rw
	s  *ServerForPick
}

// return io.EOF means server stop recv
func (c *StreamContext) Send(resp []byte) error {
	return c.rw.send(&MsgBody{Body: resp})
}
func (c *StreamContext) StopSend() {
	c.rw.closesend()
}

// return io.EOF means server stop send
func (c *StreamContext) Recv() ([]byte, error) {
	return c.rw.recv()
}
func (c *StreamContext) StopRecv() {
	c.rw.closerecv()
}
func (c *StreamContext) GetServerAddr() string {
	return c.s.GetServerAddr()
}

// ----------------------------------------------- for protobuf ------------------------------------------------------

// ----------------------------------------------- client stream context ---------------------------------------------
func NewClientStreamClientContext[reqtype any](path string, ctx *StreamContext, validatereq bool) *ClientStreamClientContext[reqtype] {
	return &ClientStreamClientContext[reqtype]{path: path, Context: ctx.Context, cctx: ctx, validatereq: validatereq}
}

type ClientStreamClientContext[reqtype any] struct {
	path        string
	validatereq bool
	context.Context
	cctx *StreamContext
}

// return io.EOF means server stop recv
func (c *ClientStreamClientContext[reqtype]) Send(req *reqtype) error {
	var tmp any = req
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] request struct's type is not proto's message")
		return cerror.ErrSystem
	}
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.path+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.cctx.Send(d)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.path+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ClientStreamClientContext[reqtype]) StopSend() {
	c.cctx.StopSend()
}
func (c *ClientStreamClientContext[reqtype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}

// ----------------------------------------------- server stream context ---------------------------------------------
func NewServerStreamClientContext[resptype any](path string, ctx *CallContext) *ServerStreamClientContext[resptype] {
	return &ServerStreamClientContext[resptype]{path: path, Context: ctx.Context, cctx: ctx}
}

type ServerStreamClientContext[resptype any] struct {
	path string
	context.Context
	cctx *CallContext
}

// return io.EOF means server stop send
func (c *ServerStreamClientContext[resptype]) Recv() (*resptype, error) {
	var resp any = new(resptype)
	m, ok := resp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] response struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, e := c.cctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.path+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	if e := proto.Unmarshal(data, m); e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] response decode failed", slog.String("error", e.Error()))
		return nil, e
	}
	return resp.(*resptype), nil
}
func (c *ServerStreamClientContext[resptype]) StopRecv() {
	c.cctx.StopRecv()
}
func (c *ServerStreamClientContext[resptype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}

// ----------------------------------------------- all stream context ------------------------------------------------
func NewAllStreamClientContext[reqtype, resptype any](path string, ctx *StreamContext, validatereq bool) *AllStreamClientContext[reqtype, resptype] {
	return &AllStreamClientContext[reqtype, resptype]{path: path, Context: ctx.Context, cctx: ctx, validatereq: validatereq}
}

type AllStreamClientContext[reqtype, resptype any] struct {
	path        string
	validatereq bool
	context.Context
	cctx *StreamContext
}

// return io.EOF means server stop recv
func (c *AllStreamClientContext[reqtype, resptype]) Send(req *reqtype) error {
	var tmp any = req
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] request struct's type is not proto's message")
		return cerror.ErrSystem
	}
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.path+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.cctx.Send(d)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.path+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamClientContext[reqtype, resptype]) StopSend() {
	c.cctx.StopSend()
}

// return io.EOF means server stop send
func (c *AllStreamClientContext[reqtype, resptype]) Recv() (*resptype, error) {
	var resp any = new(resptype)
	m, ok := resp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.path+"] response struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, e := c.cctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.path+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	if e := proto.Unmarshal(data, m); e != nil {
		slog.ErrorContext(c.Context, "["+c.path+"] response decode failed", slog.String("error", e.Error()))
		return nil, e
	}
	return resp.(*resptype), nil
}
func (c *AllStreamClientContext[reqtype, resptype]) StopRecv() {
	c.cctx.StopRecv()
}
func (c *AllStreamClientContext[reqtype, resptype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}
