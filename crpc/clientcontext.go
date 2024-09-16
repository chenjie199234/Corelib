package crpc

import (
	"context"
)

type CallContext struct {
	context.Context
	rw *rw
	s  *ServerForPick
}

func (c *CallContext) Read() ([]byte, error) {
	return c.rw.read()
}
func (c *CallContext) StopRead() {
	c.rw.closeread()
}
func (c *CallContext) GetServerAddr() string {
	return c.s.GetServerAddr()
}

type StreamContext struct {
	context.Context
	rw *rw
	s  *ServerForPick
}

func (c *StreamContext) Send(resp []byte) error {
	return c.rw.send(&MsgBody{Body: resp})
}
func (c *StreamContext) StopSend() {
	c.rw.closesend()
}
func (c *StreamContext) Read() ([]byte, error) {
	return c.rw.read()
}
func (c *StreamContext) StopRead() {
	c.rw.closeread()
}
func (c *StreamContext) GetServerAddr() string {
	return c.s.GetServerAddr()
}
