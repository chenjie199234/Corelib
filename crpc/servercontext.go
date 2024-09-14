package crpc

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
)

type ServerContext struct {
	context.Context
	context.CancelFunc
	rw       *rw
	peer     *stream.Peer
	peerip   string
	handlers []OutsideHandler
	finish   int32
	e        *cerror.Error
}

func (c *ServerContext) run() {
	for _, handler := range c.handlers {
		handler(c)
		if c.finish != 0 {
			break
		}
	}
}

func (c *ServerContext) AbortWrite(e error) error {
	if atomic.SwapInt32(&c.finish, 1) != 0 {
		return cerror.ErrStreamSendClosed
	}
	c.e = cerror.Convert(e)
	if c.e != nil && (c.e.Httpcode < 400 || c.e.Httpcode > 999) {
		panic("[crpc.Context.Abort] http code must in [400,999)")
	}
	if c.e != nil {
		traildata := map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
		return c.rw.write(context.Background(), &MsgBody{Error: c.e, Traildata: traildata})
	}
	return c.rw.closesend(context.Background())
}
func (c *ServerContext) Write(resp []byte) error {
	if atomic.LoadInt32(&c.finish) != 0 {
		return cerror.ErrStreamSendClosed
	}
	traildata := map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
	return c.rw.write(context.Background(), &MsgBody{Body: resp, Traildata: traildata})
}

// context's cancel can wake up the read's block
func (c *ServerContext) Read(ctx context.Context) ([]byte, error) {
	body, _, e := c.rw.read(ctx)
	if e != nil {
		return nil, e
	}
	return body, e
}
func (c *ServerContext) AbortRead() error {
	return c.rw.closeread(context.Background())
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

func (c *ServerContext) GetPeerMaxMsgLen() uint32 {
	return c.peer.GetPeerMaxMsgLen()
}
