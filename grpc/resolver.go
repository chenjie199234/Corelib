package grpc

import (
	"strings"

	"google.golang.org/grpc/resolver"
)

type builder struct {
	c  *GrpcClient
	cc resolver.ClientConn
}

func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	b.cc = cc
	strs := strings.Split(target.URL.Path, ".")
	manually := make(chan *struct{}, 1)
	manually <- nil
	go b.c.c.Discover(strs[0], strs[1], manually, b.c)
	return &discover{manually: manually}, nil
}

func (b *builder) Scheme() string {
	return "corelib"
}

type discover struct {
	manually chan *struct{}
}

func (d *discover) ResolveNow(op resolver.ResolveNowOptions) {
	select {
	case d.manually <- nil:
	default:
	}
}

func (d *discover) Close() {
}
