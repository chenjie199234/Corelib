package crpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/log"
)

type corelibResolver struct {
	lker         *sync.Mutex
	sstatus      bool
	system       chan *struct{}
	systemNotice map[chan *struct{}]*struct{}
	cstatus      bool
	call         chan *struct{}
	callNotice   map[chan *struct{}]*struct{}
	stop         chan *struct{}
	stopstatus   int32
}

func newCorelibResolver(group, name string, c *CrpcClient) *corelibResolver {
	r := &corelibResolver{
		lker:         &sync.Mutex{},
		sstatus:      true,
		system:       make(chan *struct{}, 1),
		systemNotice: make(map[chan *struct{}]*struct{}),
		cstatus:      false,
		call:         make(chan *struct{}, 1),
		callNotice:   make(map[chan *struct{}]*struct{}),
		stop:         make(chan *struct{}),
	}
	r.system <- nil
	go func() {
		tker := time.NewTicker(c.c.DiscoverInterval)
		for {
			select {
			case <-tker.C:
			case <-r.system:
			case <-r.call:
			case <-r.stop:
				tker.Stop()
				return
			}
			all, e := c.c.Discover(group, name)
			if e != nil {
				c.balancer.ResolverError(e)
				log.Error(nil, "[crpc.client.resolver] discover servername:", name, "servergroup:", group, e)
				r.wake(true)
				r.wake(false)
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
		if notice != nil {
			r.systemNotice[notice] = nil
		}
		if r.sstatus {
			return
		}
		r.sstatus = true
		r.system <- nil
	} else {
		if notice != nil {
			r.callNotice[notice] = nil
		}
		if r.cstatus {
			return
		}
		r.cstatus = true
		r.call <- nil
	}
}

// systemORcall true - system,false - call
func (r *corelibResolver) wait(ctx context.Context, systemORcall bool) error {
	notice := make(chan *struct{}, 1)
	r.triger(notice, systemORcall)
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		r.lker.Lock()
		if systemORcall {
			delete(r.systemNotice, notice)
		} else {
			delete(r.callNotice, notice)
		}
		r.lker.Unlock()
		return ctx.Err()
	}
}

// systemORcall true - system,false - call
func (r *corelibResolver) wake(systemORcall bool) {
	r.lker.Lock()
	if systemORcall {
		r.sstatus = false
		for notice := range r.systemNotice {
			delete(r.systemNotice, notice)
			notice <- nil
		}
	} else {
		r.cstatus = false
		for notice := range r.callNotice {
			delete(r.callNotice, notice)
			notice <- nil
		}
	}
	r.lker.Unlock()
}
