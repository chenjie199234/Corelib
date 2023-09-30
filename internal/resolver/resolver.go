package resolver

import (
	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	gresolver "google.golang.org/grpc/resolver"
)

type Balancer interface {
	ResolverError(error)
	//key:addr
	UpdateDiscovery(datas map[string]*discover.RegisterData, version discover.Version)
}

type CorelibResolver struct {
	b    Balancer
	d    discover.DI
	pt   discover.PortType
	stop chan *struct{}
}

func NewCorelibResolver(b Balancer, d discover.DI, pt discover.PortType) *CorelibResolver {
	return &CorelibResolver{
		b:    b,
		d:    d,
		pt:   pt,
		stop: make(chan *struct{}),
	}
}
func (r *CorelibResolver) Start() {
	go func() {
		dnotice, cancel := r.d.GetNotice()
		defer cancel()
		for {
			var ok bool
			select {
			case _, ok = <-dnotice:
				if !ok {
					r.b.ResolverError(cerror.ErrDiscoverStopped)
					<-r.stop
					r.b.ResolverError(cerror.ErrClientClosing)
					return
				}
			case <-r.stop:
				r.b.ResolverError(cerror.ErrClientClosing)
				return
			}
			all, version, e := r.d.GetAddrs(r.pt)
			if e != nil {
				r.b.ResolverError(e)
			} else {
				for k, v := range all {
					if v == nil || len(v.DServers) == 0 {
						delete(all, k)
					}
				}
				r.b.UpdateDiscovery(all, version)
			}
		}
	}()
}

func (r *CorelibResolver) ResolveNow(gresolver.ResolveNowOptions) {
	r.d.Now()
}

func (r *CorelibResolver) Now() {
	r.d.Now()
}
func (r *CorelibResolver) Close() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}
