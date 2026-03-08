package web

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type SSEContext struct {
	context.Context
	resp  *http.Response
	rd    *bufio.Reader
	retry int64
}

func NewSSEContext(resp *http.Response) *SSEContext {
	return &SSEContext{
		Context: resp.Request.Context(),
		resp:    resp,
		rd:      bufio.NewReader(resp.Body),
	}
}

// return io.EOF means server stop send
// return cerror.ErrClosed means connection closed
func (c *SSEContext) Recv() (id, event string, data []byte, err error) {
	var buf []byte
	for {
		line, e := c.rd.ReadBytes('\n')
		if e != nil {
			if e != io.EOF {
				// slog.ErrorContext(c.Context, "["+c.resp.Request.URL.Path+"] read SSE data field", slog.String("error", e.Error()))
				err = cerror.ErrClosed
			} else {
				err = e
			}
			return
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			//finish this event
			if event == "" {
				event = "message"
			}
			data = buf
			return
		}
		switch {
		case strings.HasPrefix(common.BTS(line), "event:"):
			//only the first space is meaningless,it can exist or not
			tmp := line[6:]
			if len(tmp) > 0 && tmp[0] == ' ' {
				tmp = tmp[1:]
			}
			event = common.BTS(tmp)
		case strings.HasPrefix(common.BTS(line), "id:"):
			//only the first space is meaningless,it can exist or not
			tmp := line[3:]
			if len(tmp) > 0 && tmp[0] == ' ' {
				tmp = tmp[1:]
			}
			id = common.BTS(tmp)
		case strings.HasPrefix(common.BTS(line), "retry:"):
			//only the first space is meaningless,it can exist or not
			tmp := line[6:]
			if len(tmp) > 0 && tmp[0] == ' ' {
				tmp = tmp[1:]
			}
			str := common.BTS(tmp)
			if c.retry, e = strconv.ParseInt(str, 10, 64); e != nil {
				slog.ErrorContext(c.Context, "["+c.resp.Request.URL.Path+"] SSE data broken for retry field", slog.String("retry", str))
			}
			if c.retry < 0 {
				c.retry = 0
			}
		case strings.HasPrefix(common.BTS(line), "data: "):
			tmp := line[5:]
			if len(tmp) > 0 && tmp[0] == ' ' {
				tmp = tmp[1:]
			}
			if len(buf) > 0 {
				//multi data field use \n to connect
				buf = append(buf, '\n')
			}
			buf = append(buf, tmp...)
		default:
			//ignore all other line
		}
	}
}

// this can be used to wake up the block on Recv
func (c *SSEContext) StopRecv() {
	c.resp.Body.Close()
}
func (c *SSEContext) GetRetryMS() int64 {
	return c.retry
}

// ----------------------------------------------- for protobuf ------------------------------------------------------

func NewServerStreamSSEContext[resptype any](ctx *SSEContext) *ServerStreamSSEContext[resptype] {
	return &ServerStreamSSEContext[resptype]{Context: ctx.Context, cctx: ctx}
}

type ServerStreamSSEContext[resptype any] struct {
	context.Context
	cctx *SSEContext
}

// return io.EOF means server stop send
// return cerror.ErrClosed means connection closed
func (c *ServerStreamSSEContext[resptype]) Recv() (string, string, *resptype, error) {
	var resp any = new(resptype)
	m, ok := resp.(protoreflect.ProtoMessage)
	if !ok {
		//if use the protoc-go-crpc's generate code,this will not happen
		slog.ErrorContext(c.Context, "["+c.cctx.resp.Request.URL.Path+"] response struct's type is not proto's message")
		return "", "", nil, cerror.ErrSystem
	}
	var data []byte
	id, event, data, e := c.cctx.Recv()
	if e != nil {
		slog.ErrorContext(c.Context, "["+c.cctx.resp.Request.URL.Path+"] read response failed", slog.String("error", e.Error()))
		return "", "", nil, e
	}
	if event == "error" && len(data) > 0 {
		return "", "", nil, cerror.Decode(common.BTS(data))
	}
	if e := protojson.Unmarshal(data, m); e != nil {
		slog.ErrorContext(c.Context, "["+c.cctx.resp.Request.URL.Path+"] decode response failed", slog.String("error", e.Error()))
		return "", "", nil, e
	}
	return id, event, resp.(*resptype), nil
}
func (c *ServerStreamSSEContext[resptype]) StopRecv() {
	c.cctx.StopRecv()
}
func (c *ServerStreamSSEContext[resptype]) GetRetryMS() int64 {
	return c.cctx.GetRetryMS()
}
