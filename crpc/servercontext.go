package crpc

import (
	"context"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/stream"
)

type ServerContext struct {
	context.Context
	cancel context.CancelFunc
	rw     *rw
	peer   *stream.Peer
	peerip string
	finish bool
	e      *cerror.Error
}

func (c *ServerContext) Abort() {
	c.finish = true
}

func (c *ServerContext) Send(resp []byte) error {
	return c.rw.send(&MsgBody{Body: resp})
}
func (c *ServerContext) StopSend(e error) {
	c.e = cerror.Convert(e)
	if c.e != nil && (c.e.Httpcode < 400 || c.e.Httpcode > 999) {
		panic("[crpc.Context.Abort] http code must in [400,999)")
	}
	if c.e != nil {
		c.rw.send(&MsgBody{Error: c.e})
	} else {
		c.rw.closesend()
	}
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

func (c *ServerContext) GetPeerMaxMsgLen() uint32 {
	return c.peer.GetPeerMaxMsgLen()
}
