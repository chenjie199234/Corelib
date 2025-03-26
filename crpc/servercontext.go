package crpc

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
	"github.com/chenjie199234/Corelib/stream"

	"google.golang.org/protobuf/encoding/protojson"
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

// means stop recv and send
func (c *ServerContext) Abort(e error) {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return
	}
	httpcode := 0
	if ee := cerror.Convert(e); ee != nil {
		if http.StatusText(int(ee.Httpcode)) == "" || ee.Httpcode < 400 {
			c.e = cerror.ErrPanic
			c.rw.send(&MsgBody{Error: c.e})
			httpcode = int(ee.Httpcode)
		} else {
			c.e = ee
			c.rw.send(&MsgBody{Error: c.e})
		}
	}
	c.rw.closerecvsend(true, c.e)
	if httpcode != 0 {
		panic("[crpc.context.Abort] unknown http code: " + strconv.Itoa(httpcode))
	}
}

// return io.EOF means client stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *ServerContext) Send(resp []byte, encoder Encoder) error {
	return c.rw.send(&MsgBody{Body: resp, BodyEncoder: encoder})
}
func (c *ServerContext) StopSend() {
	c.rw.closesend()
}

// return io.EOF means client stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *ServerContext) Recv() ([]byte, Encoder, error) {
	return c.rw.recv()
}
func (c *ServerContext) StopRecv() {
	c.rw.closerecv()
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

func NewNoStreamServerContext(ctx *ServerContext) *NoStreamServerContext {
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

// ------------------------ client stream context ------------------------------------------------------------
func NewClientStreamServerContext[reqtype any](ctx *ServerContext, validatereq bool) *ClientStreamServerContext[reqtype] {
	return &ClientStreamServerContext[reqtype]{Context: ctx.Context, sctx: ctx, validatereq: validatereq}
}

type ClientStreamServerContext[reqtype any] struct {
	validatereq bool
	context.Context
	sctx *ServerContext
}

// return io.EOF means client stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *ClientStreamServerContext[reqtype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	m, ok := req.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, encoder, e := c.sctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] read request failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	switch encoder {
	case Encoder_Protobuf:
		if e := proto.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	case Encoder_Json:
		if e := (protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}).Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	default:
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request encoder unknown")
		return nil, cerror.ErrReq
	}
	if c.validatereq {
		if v, ok := req.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request validate failed", slog.String("error", errstr))
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

// ------------------------ server stream context(this is for protobuf,so Send's encoder always be Encoder_Protobuf) -----------
func NewServerStreamServerContext[resptype any](ctx *ServerContext, encoder Encoder) *ServerStreamServerContext[resptype] {
	if encoder == Encoder_Unknown || encoder > Encoder_Json {
		return nil
	}
	return &ServerStreamServerContext[resptype]{Context: ctx.Context, sctx: ctx, encoder: encoder}
}

type ServerStreamServerContext[resptype any] struct {
	context.Context
	sctx    *ServerContext
	encoder Encoder
}

// return io.EOF means client stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *ServerStreamServerContext[resptype]) Send(resp *resptype) error {
	var tmp any = resp
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] response struct's type is not proto's message")
		return cerror.ErrSystem
	}
	var d []byte
	switch c.encoder {
	case Encoder_Protobuf:
		d, _ = proto.Marshal(tmptmp)
	case Encoder_Json:
		d, _ = (protojson.MarshalOptions{AllowPartial: true, UseProtoNames: true, UseEnumNumbers: true, EmitUnpopulated: true}).Marshal(tmptmp)
	}
	e := c.sctx.Send(d, c.encoder)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ServerStreamServerContext[resptype]) StopSend() {
	c.sctx.StopSend()
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

// ------------------------ all stream context(this is for protobuf,so Send's encoder always be Encoder_Protobuf) -----------
func NewAllStreamServerContext[reqtype, resptype any](ctx *ServerContext, validatereq bool) *AllStreamServerContext[reqtype, resptype] {
	return &AllStreamServerContext[reqtype, resptype]{sctx: ctx, Context: ctx.Context, validatereq: validatereq}
}

type AllStreamServerContext[reqtype, resptype any] struct {
	validatereq bool
	context.Context
	sctx *ServerContext
}

// return io.EOF means client stop send
// return cerror.ErrCanceled means self stop recv anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
func (c *AllStreamServerContext[reqtype, resptype]) Recv() (*reqtype, error) {
	var req any = new(reqtype)
	m, ok := req.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request struct's type is not proto's message")
		return nil, cerror.ErrSystem
	}
	data, encoder, e := c.sctx.Recv()
	if e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] read request failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	switch encoder {
	case Encoder_Protobuf:
		if e := proto.Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	case Encoder_Json:
		if e := (protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}).Unmarshal(data, m); e != nil {
			slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request decode failed", slog.String("error", e.Error()))
			return nil, e
		}
	default:
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request encoder unknown")
		return nil, cerror.ErrReq
	}
	if c.validatereq {
		if v, ok := req.(interface{ Validate() string }); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] request validate failed", slog.String("error", errstr))
				return nil, cerror.ErrReq
			}
		}
	}
	return req.(*reqtype), nil
}
func (c *AllStreamServerContext[reqtype, resptype]) StopRecv() {
	c.sctx.StopRecv()
}

// return io.EOF means client stop recv
// return cerror.ErrCanceled means self stop send anymore in this Context
// return cerror.ErrClosed means connection between client and server is closed
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (c *AllStreamServerContext[reqtype, resptype]) Send(resp *resptype) error {
	var tmp any = resp
	tmptmp, ok := tmp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] response struct's type is not proto's message")
		return cerror.ErrSystem
	}
	d, _ := proto.Marshal(tmptmp)
	e := c.sctx.Send(d, Encoder_Protobuf)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.Context, "["+c.sctx.GetPath()+"] send response failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamServerContext[reqtype, resptype]) StopSend() {
	c.sctx.StopSend()
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
