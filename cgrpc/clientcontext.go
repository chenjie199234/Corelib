package cgrpc

import (
	"io"
	"log/slog"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pbex"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// ----------------------------------------------- client stream context ---------------------------------------------
func NewClientStreamClientContext[reqtype any](path string, stream grpc.ClientStream, validatereq bool) *ClientStreamClientContext[reqtype] {
	return &ClientStreamClientContext[reqtype]{
		validatereq: validatereq,
		path:        path,
		stream:      stream,
	}
}

type ClientStreamClientContext[reqtype any] struct {
	validatereq bool
	path        string
	stream      grpc.ClientStream
}

func (c *ClientStreamClientContext[reqtype]) Send(req *reqtype) error {
	var tmp any = req
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.stream.Context(), "["+c.path+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	e := c.stream.SendMsg(req)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.stream.Context(), "["+c.path+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *ClientStreamClientContext[reqtype]) GetServerAddr() string {
	p, ok := peer.FromContext(c.stream.Context())
	if !ok {
		return ""
	}
	return p.Addr.String()
}

// ----------------------------------------------- server stream context ---------------------------------------------
func NewServerStreamClientContext[resptype any](path string, stream grpc.ClientStream) *ServerStreamClientContext[resptype] {
	return &ServerStreamClientContext[resptype]{
		path:   path,
		stream: stream,
	}
}

type ServerStreamClientContext[resptype any] struct {
	path   string
	stream grpc.ClientStream
}

func (c *ServerStreamClientContext[resptype]) Recv() (*resptype, error) {
	resp := new(resptype)
	if e := c.stream.RecvMsg(resp); e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.stream.Context(), "["+c.path+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	return resp, nil
}
func (c *ServerStreamClientContext[resptype]) GetServerAddr() string {
	p, ok := peer.FromContext(c.stream.Context())
	if !ok {
		return ""
	}
	return p.Addr.String()
}

// ----------------------------------------------- all stream context ---------------------------------------------
func NewAllStreamClientContext[reqtype, resptype any](path string, stream grpc.ClientStream, validatereq bool) *AllStreamClientContext[reqtype, resptype] {
	return &AllStreamClientContext[reqtype, resptype]{
		path:        path,
		validatereq: validatereq,
		stream:      stream,
	}
}

type AllStreamClientContext[reqtype, resptype any] struct {
	path        string
	validatereq bool
	stream      grpc.ClientStream
}

func (c *AllStreamClientContext[reqtype, resptype]) Recv() (*resptype, error) {
	resp := new(resptype)
	if e := c.stream.RecvMsg(resp); e != nil {
		if e != io.EOF {
			slog.ErrorContext(c.stream.Context(), "["+c.path+"] read response failed", slog.String("error", e.Error()))
		}
		return nil, e
	}
	return resp, nil
}
func (c *AllStreamClientContext[reqtype, resptype]) Send(req *reqtype) error {
	var tmp any = req
	if c.validatereq {
		if v, ok := tmp.(pbex.Validater); ok {
			if errstr := v.Validate(); errstr != "" {
				slog.ErrorContext(c.stream.Context(), "["+c.path+"] request validate failed", slog.String("error", errstr))
				return cerror.ErrReq
			}
		}
	}
	e := c.stream.SendMsg(req)
	if e != nil && e != io.EOF {
		slog.ErrorContext(c.stream.Context(), "["+c.path+"] send request failed", slog.String("error", e.Error()))
	}
	return e
}
func (c *AllStreamClientContext[reqtype, resptype]) GetServerAddr() string {
	p, ok := peer.FromContext(c.stream.Context())
	if !ok {
		return ""
	}
	return p.Addr.String()
}
