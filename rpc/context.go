package rpc

import (
	"context"

	"github.com/chenjie199234/Corelib/sys/cpu"
)

type Context struct {
	context.Context
	msg            *Msg
	peeruniquename string
	handlers       []OutsideHandler
	next           int8
}

func (c *Context) Next() {
	if c.next < 0 {
		return
	}
	c.next++
	for c.next < int8(len(c.handlers)) {
		c.handlers[c.next](c)
		if c.next < 0 {
			break
		}
		c.next++
	}
}

func (c *Context) Abort(e error) {
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = nil
	c.msg.Cpu = cpu.GetUse()
	c.msg.Error = e.Error()
	c.msg.Metadata = nil
	c.next = -1
}

func (c *Context) Write(resp []byte) {
	c.msg.Path = ""
	c.msg.Deadline = 0
	c.msg.Body = resp
	c.msg.Cpu = cpu.GetUse()
	c.msg.Error = ""
	c.msg.Metadata = nil
	c.next = -1
}
func (c *Context) GetBody() []byte {
	return c.msg.Body
}
func (c *Context) GetPeerName() string {
	return c.peeruniquename
}
func (c *Context) GetPath() string {
	return c.msg.Path
}
func (c *Context) GetDeadline() int64 {
	return c.msg.Deadline
}
func (c *Context) GetMetadata() map[string]string {
	return c.msg.Metadata
}
