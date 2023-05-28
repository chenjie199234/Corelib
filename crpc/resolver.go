package crpc

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/log"
)

type corelibResolver struct {
	c          *CrpcClient
	lker       *sync.Mutex
	snotices   map[chan *struct{}]*struct{}
	cnotices   map[chan *struct{}]*struct{}
	stop       chan *struct{}
	stopstatus int32
}

func newCorelibResolver(c *CrpcClient) *corelibResolver {
	r := &corelibResolver{
		c:        c,
		lker:     &sync.Mutex{},
		snotices: make(map[chan *struct{}]*struct{}, 10),
		cnotices: make(map[chan *struct{}]*struct{}, 10),
		stop:     make(chan *struct{}),
	}
	go func() {
		for {
			dnotice, cancel := c.discover.GetNotice()
			defer cancel()
			//first init triger,this is used to active the dnotice
			c.discover.Now()
			for {
				select {
				case _, ok := <-dnotice:
					if !ok {
						log.Error(nil, "[crpc.client.resolver] discover stopped!")
					}
				case <-r.stop:
					c.balancer.ResolverError(ClientClosed)
					r.wake(false)
					r.wake(true)
					return
				}
				all, e := c.discover.GetAddrs(discover.Crpc)
				if e != nil {
					c.balancer.ResolverError(e)
					r.wake(false)
					r.wake(true)
				} else {
					for k, v := range all {
						if v == nil || len(v.DServers) == 0 {
							delete(all, k)
						}
					}
					c.balancer.UpdateDiscovery(all)
				}
			}
		}
	}()
	return r
}

func (r *corelibResolver) ResolveNow() {
	r.triger(nil, true)
}
func (r *corelibResolver) Close() {
	if atomic.SwapInt32(&r.stopstatus, 1) == 1 {
		return
	}
	close(r.stop)
}

// systemORcall true - system,false - call
func (r *corelibResolver) triger(notice chan *struct{}, systemORcall bool) {
	r.lker.Lock()
	defer r.lker.Unlock()
	if systemORcall {
		exist := len(r.snotices)
		r.snotices[notice] = nil
		if exist == 0 {
			r.c.discover.Now()
		}
	} else {
		exist := len(r.cnotices)
		r.cnotices[notice] = nil
		if exist == 0 {
			r.c.discover.Now()
		}
	}
}

// systemORcall true - system,false - call
// the system's wait will block until the discover return,no matter there are usable servers or not
// the call's wail will block until there are usable servers
func (r *corelibResolver) wait(ctx context.Context, systemORcall bool) error {
	notice := make(chan *struct{}, 1)
	r.triger(notice, systemORcall)
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		r.lker.Lock()
		if systemORcall {
			delete(r.snotices, notice)
		} else {
			delete(r.cnotices, notice)
		}
		r.lker.Unlock()
		return ctx.Err()
	}
}

// systemORcall true - system,false - call
func (r *corelibResolver) wake(systemORcall bool) {
	r.lker.Lock()
	if systemORcall {
		for notice := range r.snotices {
			delete(r.snotices, notice)
			if notice != nil {
				notice <- nil
			}
		}
	} else {
		for notice := range r.cnotices {
			delete(r.cnotices, notice)
			if notice != nil {
				notice <- nil
			}
		}
	}
	r.lker.Unlock()
}
