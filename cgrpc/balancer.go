package cgrpc

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/log"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	gresolver "google.golang.org/grpc/resolver"
)

// ---------------------------------------------------------------------------------------------------------------------------------------------
type resolverBuilder struct {
	c *CGrpcClient
}

func (b *resolverBuilder) Build(target gresolver.Target, cc gresolver.ClientConn, opts gresolver.BuildOptions) (gresolver.Resolver, error) {
	r := resolver.NewCorelibResolver(&balancerWraper{cc: cc}, b.c.discover)
	b.c.resolver = r
	return r, nil
}

func (b *resolverBuilder) Scheme() string {
	return "corelib"
}

// ---------------------------------------------------------------------------------------------------------------------------------------------
type balancerWraper struct {
	cc gresolver.ClientConn
}

func (b *balancerWraper) ResolverError(e error) {
	b.cc.ReportError(e)
}
func (b *balancerWraper) UpdateDiscovery(all map[string]*discover.RegisterData) {
	s := gresolver.State{
		Addresses: make([]gresolver.Address, 0, len(all)),
	}
	for addr, info := range all {
		if info == nil || len(info.DServers) == 0 {
			continue
		}
		attr := &attributes.Attributes{}
		attr = attr.WithValue("addition", info.Addition)
		attr = attr.WithValue("dservers", info.DServers)
		s.Addresses = append(s.Addresses, gresolver.Address{
			Addr:               addr,
			BalancerAttributes: attr,
		})
	}
	b.cc.UpdateState(s)
}

// ---------------------------------------------------------------------------------------------------------------------------------------------
type balancerBuilder struct {
	c *CGrpcClient
}

func (b *balancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	b.c.balancer = &corelibBalancer{
		c:       b.c,
		cc:      cc,
		servers: make(map[balancer.SubConn]*ServerForPick),
	}
	return b.c.balancer
}

func (b *balancerBuilder) Name() string {
	return "corelib"
}

type corelibBalancer struct {
	c                *CGrpcClient
	cc               balancer.ClientConn
	servers          map[balancer.SubConn]*ServerForPick
	pservers         []*ServerForPick
	lastResolveError error
}

type ServerForPick struct {
	addr     string
	subconn  balancer.SubConn
	dservers map[string]*struct{} //this app registered on which discovery server
	status   int32
	closing  bool

	Pickinfo *pickinfo
}

type pickinfo struct {
	LastFailTime   int64  //last fail timestamp nano second
	Activecalls    int32  //current active calls
	DServerNum     int32  //this server registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

func (s *ServerForPick) Pickable() bool {
	return s.status == int32(connectivity.Ready) && !s.closing
}

// UpdateClientConnState and UpdateSubConnState and Close and ResolverError are sync in grpc's ccBalancerWrapper's watcher in balancer_conn_wrappers.go
func (b *corelibBalancer) UpdateClientConnState(ss balancer.ClientConnState) error {
	b.lastResolveError = nil
	defer func() {
		if len(b.servers) == 0 {
			b.c.resolver.Wake(resolver.CALL)
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Idle, Picker: b})
		} else if len(b.pservers) > 0 {
			b.c.resolver.Wake(resolver.CALL)
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Ready, Picker: b})
		} else {
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Connecting, Picker: b})
		}
		b.c.resolver.Wake(resolver.SYSTEM)
	}()
	//offline
	for _, server := range b.servers {
		find := false
		for _, addr := range ss.ResolverState.Addresses {
			if addr.Addr == server.addr {
				find = true
				break
			}
		}
		if !find {
			server.dservers = nil
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().UnixNano()
		}
	}
	//online or update
	for _, addr := range ss.ResolverState.Addresses {
		dservers, _ := addr.BalancerAttributes.Value("dservers").(map[string]*struct{})
		addition, _ := addr.BalancerAttributes.Value("addition").([]byte)
		var server *ServerForPick
		for _, v := range b.servers {
			if v.addr == addr.Addr {
				server = v
				break
			}
		}
		if server == nil {
			//this is a new register
			if len(dservers) == 0 {
				continue
			}
			sc, e := b.cc.NewSubConn([]gresolver.Address{addr}, balancer.NewSubConnOptions{HealthCheckEnabled: true})
			if e != nil {
				//this can only happened on client is closing
				continue
			}
			b.servers[sc] = &ServerForPick{
				addr:     addr.Addr,
				subconn:  sc,
				dservers: dservers,
				status:   int32(connectivity.Idle),
				Pickinfo: &pickinfo{
					LastFailTime:   0,
					Activecalls:    0,
					DServerNum:     int32(len(dservers)),
					DServerOffline: 0,
					Addition:       addition,
				},
			}
			//subconn's Connect is async inside
			sc.Connect()
		} else if len(dservers) == 0 {
			//this is not a new register and this register is offline
			server.dservers = nil
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().UnixNano()
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range server.dservers {
				if _, ok := dservers[dserver]; !ok {
					server.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range dservers {
				if _, ok := server.dservers[dserver]; !ok {
					server.Pickinfo.DServerOffline = 0
					break
				}
			}
			server.dservers = dservers
			server.Pickinfo.Addition = addition
			server.Pickinfo.DServerNum = int32(len(dservers))
		}
	}
	return nil
}

// UpdateClientConnState and UpdateSubConnState and Close and ResolverError are sync in grpc's ccBalancerWrapper's watcher in balancer_conn_wrappers.go
func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
}

// UpdateClientConnState and UpdateSubConnState and Close and ResolverError are sync in grpc's ccBalancerWrapper's watcher in balancer_conn_wrappers.go
func (b *corelibBalancer) UpdateSubConnState(sc balancer.SubConn, s balancer.SubConnState) {
	server, ok := b.servers[sc]
	if !ok {
		return
	}
	defer func() {
		if len(b.servers) == 0 {
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Idle, Picker: b})
		} else if len(b.pservers) > 0 {
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Ready, Picker: b})
		} else {
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Connecting, Picker: b})
		}
	}()
	if s.ConnectivityState == connectivity.Shutdown {
		if server.status == int32(connectivity.Ready) {
			//offline
			log.Info(nil, "[cgrpc.client] server:", b.c.serverapp+":"+server.addr, "offline")
			server.status = int32(connectivity.Shutdown)
			b.rebuildpicker(false)
		} else {
			server.status = int32(connectivity.Shutdown)
		}
		delete(b.servers, sc)
		return
	}
	if s.ConnectivityState == connectivity.Idle {
		if server.status == int32(connectivity.Ready) {
			//offline
			log.Info(nil, "[cgrpc.client] server:", b.c.serverapp+":"+server.addr, "offline")
			server.status = int32(s.ConnectivityState)
			b.rebuildpicker(false)
		} else {
			server.status = int32(s.ConnectivityState)
		}
		if len(server.dservers) == 0 {
			server.status = int32(connectivity.Shutdown)
			delete(b.servers, sc)
			b.cc.RemoveSubConn(sc)
		} else {
			//subconn's Connect is async inside
			sc.Connect()
		}
	} else if s.ConnectivityState == connectivity.Ready {
		//online
		log.Info(nil, "[cgrpc.client] server:", b.c.serverapp+":"+server.addr, "online")
		server.status = int32(s.ConnectivityState)
		b.rebuildpicker(true)
	} else if s.ConnectivityState == connectivity.TransientFailure {
		//connect failed
		log.Error(nil, "[cgrpc.client] connect to server:", b.c.serverapp+":"+server.addr, s.ConnectionError)
		server.status = int32(s.ConnectivityState)
	}
}

// OnOff - true,online
// OnOff - false,offline
func (b *corelibBalancer) rebuildpicker(OnOff bool) {
	tmp := make([]*ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if server.Pickable() {
			tmp = append(tmp, server)
		}
	}
	b.pservers = tmp
	if OnOff {
		b.c.resolver.Wake(resolver.CALL)
	}
	return
}

// UpdateClientConnState and UpdateSubConnState and Close and ResolverError are sync in grpc's ccBalancerWrapper's watcher in balancer_conn_wrappers.go
func (b *corelibBalancer) Close() {
	for _, server := range b.servers {
		server.status = int32(connectivity.Shutdown)
		b.cc.RemoveSubConn(server.subconn)
		log.Info(nil, "[cgrpc.client] server:", b.c.serverapp+":"+server.addr, "offline")
	}
	b.servers = make(map[balancer.SubConn]*ServerForPick)
	b.pservers = make([]*ServerForPick, 0)
}

func (b *corelibBalancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	refresh := false
	for {
		server := b.c.picker(b.pservers)
		if server != nil {
			if dl, ok := info.Ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				//at least 5ms for net lag and server logic
				return balancer.PickResult{}, cerror.ErrDeadlineExceeded
			}
			atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
			return balancer.PickResult{
				SubConn: server.subconn,
				Done: func(doneinfo balancer.DoneInfo) {
					atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
					if doneinfo.Err != nil {
						server.Pickinfo.LastFailTime = time.Now().UnixNano()
						if cerror.Equal(transGrpcError(doneinfo.Err), cerror.ErrServerClosing) || cerror.Equal(transGrpcError(doneinfo.Err), cerror.ErrTarget) {
							server.closing = true
							b.c.ResolveNow()
						}
					}
				},
			}, nil
		}
		if refresh {
			if b.lastResolveError != nil {
				return balancer.PickResult{}, b.lastResolveError
			}
			return balancer.PickResult{}, cerror.ErrNoserver
		}
		if e := b.c.resolver.Wait(info.Ctx, resolver.CALL); e != nil {
			if e == context.DeadlineExceeded {
				return balancer.PickResult{}, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return balancer.PickResult{}, cerror.ErrCanceled
			} else {
				//this is impossible
				return balancer.PickResult{}, cerror.ConvertStdError(e)
			}
		}
		refresh = true
	}
}
