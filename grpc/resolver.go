package grpc

import (
	"strings"

	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	c *GrpcClient
}

func (b *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	b.c.resolver = &corelibResolver{
		manually: make(chan *struct{}, 1),
		cc:       cc,
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
	manually chan *struct{}
	cc       resolver.ClientConn
}

func (r *corelibResolver) ResolveNow(op resolver.ResolveNowOptions) {
	select {
	case r.manually <- nil:
	default:
	}
}

func (r *corelibResolver) Close() {
}
