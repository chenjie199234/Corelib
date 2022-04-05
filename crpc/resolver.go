package crpc

import (
	"context"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
)

type corelibResolver struct {
	lker         *sync.Mutex
	mstatus      bool
	manually     chan *struct{}
	manualNotice map[chan *struct{}]*struct{}
}

func newCorelibResolver(group, name string, c *CrpcClient) *corelibResolver {
	r := &corelibResolver{
		lker:         &sync.Mutex{},
		mstatus:      true,
		manually:     make(chan *struct{}, 1),
		manualNotice: make(map[chan *struct{}]*struct{}),
	}
	r.manually <- nil
	go func() {
		tker := time.NewTicker(c.c.DiscoverInterval)
		for {
			select {
			case <-tker.C:
			case <-r.manually:
			}
			all, e := c.c.Discover(group, name)
			if e != nil {
				c.balancer.ResolverError(e)
				log.Error(nil, "[crpc.client.resolver] discover servername:", name, "servergroup:", group, "error:", e)
				r.wakemanual()
				continue
			}
			for k, v := range all {
				if v == nil || len(v.DServers) == 0 {
					delete(all, k)
				}
			}
			c.balancer.UpdateDiscovery(all)
		}
	}()
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
	if c.mstatus {
		c.mstatus = false
		for notice := range c.manualNotice {
			delete(c.manualNotice, notice)
			notice <- nil
		}
	}
	c.lker.Unlock()
}
