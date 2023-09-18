package resolver

import (
	"context"
	"sync"
	"sync/atomic"

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
	b          Balancer
	d          discover.DI
	lker       *sync.Mutex
	notices    map[WaitType]map[chan *struct{}]*struct{}
	stop       chan *struct{}
	stopstatus int32
}

func NewCorelibResolver(b Balancer, d discover.DI, pt discover.PortType) *CorelibResolver {
	r := &CorelibResolver{
		b:       b,
		d:       d,
		lker:    &sync.Mutex{},
		notices: make(map[WaitType]map[chan *struct{}]*struct{}, 5),
		stop:    make(chan *struct{}),
	}
	go func() {
		for {
			dnotice, cancel := d.GetNotice()
			defer cancel()
			for {
				var ok bool
				select {
				case _, ok = <-dnotice:
					if !ok {
						b.ResolverError(cerror.ErrDiscoverStopped)
						r.Wake(CALL)
						r.Wake(SYSTEM)
						<-r.stop
						b.ResolverError(cerror.ErrClientClosing)
						r.Wake(CALL)
						r.Wake(SYSTEM)
					}
				case <-r.stop:
					b.ResolverError(cerror.ErrClientClosing)
					r.Wake(CALL)
					r.Wake(SYSTEM)
					return
				}
				all, version, e := d.GetAddrs(pt)
				if e != nil {
					b.ResolverError(e)
					r.Wake(CALL)
					r.Wake(SYSTEM)
				} else {
					for k, v := range all {
						if v == nil || len(v.DServers) == 0 {
							delete(all, k)
						}
					}
					b.UpdateDiscovery(all, version)
				}
			}
		}
	}()
	return r
}
func (r *CorelibResolver) ResolveNow(gresolver.ResolveNowOptions) {
	r.triger(nil, SYSTEM)
}
func (r *CorelibResolver) Now() {
	r.triger(nil, SYSTEM)
}
func (r *CorelibResolver) Close() {
	if atomic.SwapInt32(&r.stopstatus, 1) == 1 {
		return
	}
	close(r.stop)
}

type WaitType uint8

const (
	// the system's wait should block until the discover return,no matter there are usable servers or not
	SYSTEM WaitType = iota
	// the call's wail should block until there are usable servers
	CALL
)

func (r *CorelibResolver) triger(notice chan *struct{}, wt WaitType) {
	r.lker.Lock()
	defer r.lker.Unlock()
	notices, ok := r.notices[wt]
	if !ok {
		notices = make(map[chan *struct{}]*struct{}, 10)
		r.notices[wt] = notices
	}
	exist := len(notices)
	notices[notice] = nil
	if exist == 0 {
		r.d.Now()
	}
}

func (r *CorelibResolver) Wait(ctx context.Context, wt WaitType) error {
	notice := make(chan *struct{}, 1)
	r.triger(notice, wt)
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		r.lker.Lock()
		delete(r.notices[wt], notice)
		r.lker.Unlock()
		return ctx.Err()
	}
}

func (r *CorelibResolver) Wake(wt WaitType) {
	r.lker.Lock()
	defer r.lker.Unlock()
	for notice := range r.notices[wt] {
		delete(r.notices[wt], notice)
		if notice != nil {
			notice <- nil
		}
	}
}
