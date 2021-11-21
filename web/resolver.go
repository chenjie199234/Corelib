package web

import (
	"context"
	"sync"
)

type corelibResolver struct {
	c            *WebClient
	lker         *sync.Mutex
	mstatus      bool
	manually     chan *struct{}
	manualNotice map[chan *struct{}]*struct{}
}

func newCorelibResolver(group, name string, c *WebClient) *corelibResolver {
	r := &corelibResolver{
		c:            c,
		lker:         &sync.Mutex{},
		mstatus:      true,
		manually:     make(chan *struct{}, 1),
		manualNotice: make(map[chan *struct{}]*struct{}),
	}
	r.manually <- nil
	go r.c.c.Discover(group, name, r.manually, c)
	return r
}
func (c *corelibResolver) manual(notice chan *struct{}) {
	c.lker.Lock()
	if notice != nil {
		c.manualNotice[notice] = nil
	}
	if !c.mstatus {
		c.mstatus = true
		c.manually <- nil
	}
	c.lker.Unlock()
}
func (c *corelibResolver) waitmanual(ctx context.Context) error {
	notice := make(chan *struct{}, 1)
	c.manual(notice)
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		c.lker.Lock()
		delete(c.manualNotice, notice)
		c.lker.Unlock()
		return ctx.Err()
	}
}
func (c *corelibResolver) wakemanual() {
	c.lker.Lock()
	for notice := range c.manualNotice {
		if notice != nil {
			notice <- nil
		}
		delete(c.manualNotice, notice)
	}
	c.mstatus = false
	c.lker.Unlock()
}
