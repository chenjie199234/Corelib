package crpc

import (
	"context"
	"io"
	"log/slog"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallContext struct {
	context.Context
	rw *rw
	s  *ServerForPick
}

func (c *CallContext) GetPath() string {
	return c.rw.path
}

// return io.EOF means server stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *CallContext) Recv() ([]byte, Encoder, error) {
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

func (c *StreamContext) GetPath() string {
	return c.rw.path
}

// return io.EOF means server stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *StreamContext) Send(req []byte, encoder Encoder) error {
	if encoder == Encoder_Unknown || encoder > Encoder_Json {
		return cerror.ErrReq
	}
	return c.rw.send(&MsgBody{Body: req, BodyEncoder: encoder})
}
func (c *StreamContext) StopSend() {
	c.rw.closesend()
}

// return io.EOF means server stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *StreamContext) Recv() ([]byte, Encoder, error) {
	return c.rw.recv()
}
func (c *StreamContext) StopRecv() {
	c.rw.closerecv()
}
func (c *StreamContext) GetServerAddr() string {
	return c.s.GetServerAddr()
}

// ----------------------------------------------- for protobuf ------------------------------------------------------

// ------------------------ client stream context(this is for protobuf,so Send's encoder always be Encoder_Protobuf) -----------
func NewClientStreamClientContext[reqtype any](ctx *StreamContext, validatereq bool) *ClientStreamClientContext[reqtype] {
	return &ClientStreamClientContext[reqtype]{Context: ctx.Context, cctx: ctx, validatereq: validatereq}
}

type ClientStreamClientContext[reqtype any] struct {
	validatereq bool
	context.Context
	cctx *StreamContext
}

// return io.EOF means server stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *ClientStreamClientContext[reqtype]) Send(req *reqtype) error {
	var tmp any = req
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] request struct's type is not proto's message")
		return cerror.ErrSystem
	}
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.cctx.Send(d, Encoder_Protobuf)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ClientStreamClientContext[reqtype]) StopSend() {
	c.cctx.StopSend()
}
func (c *ClientStreamClientContext[reqtype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}

// ----------------------- server stream context ---------------------------------------------
func NewServerStreamClientContext[resptype any](ctx *CallContext) *ServerStreamClientContext[resptype] {
	return &ServerStreamClientContext[resptype]{Context: ctx.Context, cctx: ctx}
}

type ServerStreamClientContext[resptype any] struct {
	context.Context
	cctx *CallContext
}

// return io.EOF means server stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *ServerStreamClientContext[resptype]) Recv() (*resptype, error) {
	var resp any = new(resptype)
	m, ok := resp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, encoder, e := c.cctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	switch encoder {
	case Encoder_Protobuf:
		if e := proto.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	case Encoder_Json:
		if e := protojson.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	default:
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response encoder unknown")
		return nil, cerror.ErrResp
	}
	return resp.(*resptype), nil
}
func (c *ServerStreamClientContext[resptype]) StopRecv() {
	c.cctx.StopRecv()
}
func (c *ServerStreamClientContext[resptype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}

// ------------------------ all stream context(this is for protobuf,so Send's encoder always be Encoder_Protobuf) -----------
func NewAllStreamClientContext[reqtype, resptype any](ctx *StreamContext, validatereq bool) *AllStreamClientContext[reqtype, resptype] {
	return &AllStreamClientContext[reqtype, resptype]{Context: ctx.Context, cctx: ctx, validatereq: validatereq}
}

type AllStreamClientContext[reqtype, resptype any] struct {
	validatereq bool
	context.Context
	cctx *StreamContext
}

// return io.EOF means server stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *AllStreamClientContext[reqtype, resptype]) Send(req *reqtype) error {
	var tmp any = req
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] request struct's type is not proto's message")
		return cerror.ErrSystem
	}
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.cctx.Send(d, Encoder_Protobuf)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamClientContext[reqtype, resptype]) StopSend() {
	c.cctx.StopSend()
}

// return io.EOF means server stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *AllStreamClientContext[reqtype, resptype]) Recv() (*resptype, error) {
	var resp any = new(resptype)
	m, ok := resp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, encoder, e := c.cctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	switch encoder {
	case Encoder_Protobuf:
		if e := proto.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	case Encoder_Json:
		if e := protojson.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	default:
		slog.ErrorContext(c.Context, "["+c.cctx.GetPath()+"] response encoder unknown")
		return nil, cerror.ErrResp
	}
	return resp.(*resptype), nil
}
func (c *AllStreamClientContext[reqtype, resptype]) StopRecv() {
	c.cctx.StopRecv()
}
func (c *AllStreamClientContext[reqtype, resptype]) GetServerAddr() string {
	return c.cctx.GetServerAddr()
}
