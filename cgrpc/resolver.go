package cgrpc

import (
	"context"
	"strings"
	"sync"

	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	c *CGrpcClient
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
	strs := strings.Split(target.URL.Path, ".")
	go b.c.c.Discover(strs[0], strs[1], b.c.resolver.manually, b.c)
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
	for notice := range r.manualNotice {
		notice <- nil
		delete(r.manualNotice, notice)
	}
	r.mstatus = false
	r.lker.Unlock()
}

func (r *corelibResolver) Close() {
}
