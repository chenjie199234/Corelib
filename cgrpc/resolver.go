package cgrpc

import (
	"context"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	group, name string
	c           *CGrpcClient
}

func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &corelibResolver{
		lker:         &sync.Mutex{},
		sstatus:      true,
		system:       make(chan *struct{}),
		systemNotice: make(map[chan *struct{}]*struct{}),
		cstatus:      false,
		call:         make(chan *struct{}, 1),
		callNotice:   make(map[chan *struct{}]*struct{}),
		cc:           cc,
	}
	b.c.resolver = r
	r.system <- nil
	go func() {
		tker := time.NewTicker(b.c.c.DiscoverInterval)
		for {
			select {
			case <-tker.C:
			case <-r.system:
			case <-r.call:
			}
			all, e := b.c.c.Discover(b.group, b.name)
			if e != nil {
				cc.ReportError(e)
				log.Error(nil, "[cgrpc.client.resolver] discover servername:", b.name, "servergroup:", b.group, "error:", e)
				b.c.resolver.wake(true)
				b.c.resolver.wake(false)
				continue
			}
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
	}()
	return b.c.resolver, nil
}

func (b *resolverBuilder) Scheme() string {
	return "corelib"
}

type corelibResolver struct {
	lker         *sync.Mutex
	sstatus      bool
	system       chan *struct{}
	systemNotice map[chan *struct{}]*struct{}
	cstatus      bool
	call         chan *struct{}
	callNotice   map[chan *struct{}]*struct{}
	cc           resolver.ClientConn
}

func (r *corelibResolver) ResolveNow(op resolver.ResolveNowOptions) {
	r.triger(nil, true)
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

func (r *corelibResolver) Close() {
}
