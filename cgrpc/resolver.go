package cgrpc

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/log"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	c *CGrpcClient
}

func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &corelibResolver{
		c:        b.c,
		lker:     &sync.Mutex{},
		snotices: make(map[chan *struct{}]*struct{}),
		cnotices: make(map[chan *struct{}]*struct{}),
		stop:     make(chan *struct{}),
	}
	b.c.resolver = r
	go func() {
		dnotice, cancel := b.c.discover.GetNotice()
		defer cancel()
		for {
			select {
			case _, ok := <-dnotice:
				if !ok {
					log.Error(nil, "[cgrpc.client.resolver] discover stopped!")
				}
			case <-r.stop:
				cc.ReportError(ClientClosed)
				b.c.resolver.wake(false)
				b.c.resolver.wake(true)
				return
			}
			all, e := b.c.discover.GetAddrs(discover.Cgrpc)
			if e != nil {
				cc.ReportError(e)
				b.c.resolver.wake(true)
				b.c.resolver.wake(false)
			} else {
				s := resolver.State{
					Addresses: make([]resolver.Address, 0, len(all)),
				}
				for addr, info := range all {
					if info == nil || len(info.DServers) == 0 {
						continue
					}
					attr := &attributes.Attributes{}
					attr = attr.WithValue("addition", info.Addition)
					attr = attr.WithValue("dservers", info.DServers)
					s.Addresses = append(s.Addresses, resolver.Address{
						Addr:               addr,
						BalancerAttributes: attr,
					})
				}
				cc.UpdateState(s)
			}
		}
	}()
	return r, nil
}

func (b *resolverBuilder) Scheme() string {
	return "corelib"
}

type corelibResolver struct {
	c          *CGrpcClient
	lker       *sync.Mutex
	snotices   map[chan *struct{}]*struct{}
	cnotices   map[chan *struct{}]*struct{}
	stop       chan *struct{}
	stopstatus int32
}

func (r *corelibResolver) ResolveNow(op resolver.ResolveNowOptions) {
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
		defer r.lker.Unlock()
		if systemORcall {
			delete(r.snotices, notice)
		} else {
			delete(r.cnotices, notice)
		}
		return ctx.Err()
	}
}

// systemORcall true - system,false - call
func (r *corelibResolver) wake(systemORcall bool) {
	r.lker.Lock()
	defer r.lker.Unlock()
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
}
