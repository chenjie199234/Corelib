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
	b.c.resolver = &corelibResolver{
		lker:         &sync.Mutex{},
		mstatus:      true,
		manually:     make(chan *struct{}, 1),
		manualNotice: make(map[chan *struct{}]*struct{}),
		cc:           cc,
	}
	b.c.resolver.manually <- nil
	go func() {
		tker := time.NewTicker(b.c.c.DiscoverInterval)
		for {
			select {
			case <-tker.C:
			case <-b.c.resolver.manually:
			}
			all, e := b.c.c.Discover(b.group, b.name)
			if e != nil {
				cc.ReportError(e)
				log.Error(nil, "[cgrpc.client.resolver] discover servername:", b.name, "servergroup:", b.group, "error:", e)
				b.c.resolver.wakemanual()
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
	mstatus      bool
	manually     chan *struct{}
	manualNotice map[chan *struct{}]*struct{}
	cc           resolver.ClientConn
}

func (r *corelibResolver) ResolveNow(op resolver.ResolveNowOptions) {
	r.manual(nil)
}

func (r *corelibResolver) manual(notice chan *struct{}) {
	r.lker.Lock()
	if notice != nil {
		r.manualNotice[notice] = nil
	}
	if !r.mstatus {
		r.mstatus = true
		r.manually <- nil
	}
	r.lker.Unlock()
}

func (r *corelibResolver) waitmanual(ctx context.Context) error {
	notice := make(chan *struct{}, 1)
	r.manual(notice)
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		r.lker.Lock()
		delete(r.manualNotice, notice)
		r.lker.Unlock()
		return ctx.Err()
	}
}
func (r *corelibResolver) wakemanual() {
	r.lker.Lock()
	if r.mstatus {
		r.mstatus = false
		for notice := range r.manualNotice {
			delete(r.manualNotice, notice)
			notice <- nil
		}
	}
	r.lker.Unlock()
}

func (r *corelibResolver) Close() {
}
