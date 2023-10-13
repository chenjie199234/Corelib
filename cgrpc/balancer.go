package cgrpc

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/waitwake"

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
	b.c.resolver = resolver.NewCorelibResolver(&balancerWraper{cc: cc}, b.c.discover, discover.Cgrpc)
	b.c.resolver.Start()
	return b.c.resolver, nil
}

func (b *resolverBuilder) Scheme() string {
	return "corelib"
}

// ---------------------------------------------------------------------------------------------------------------------------------------------
type balancerWraper struct {
	cc      gresolver.ClientConn
	version discover.Version
}

func (b *balancerWraper) ResolverError(e error) {
	b.cc.ReportError(e)
}

// version can be int64 or string(should only be used with != or ==)
func (b *balancerWraper) UpdateDiscovery(all map[string]*discover.RegisterData, version discover.Version) {
	if discover.SameVersion(b.version, version) {
		return
	}
	b.version = version
	s := gresolver.State{
		Endpoints: make([]gresolver.Endpoint, 0, 1),
	}
	serverattr := &attributes.Attributes{}
	serveraddrs := make([]gresolver.Address, 0, len(all))
	for addr, info := range all {
		if info == nil || len(info.DServers) == 0 {
			continue
		}
		addrattr := &attributes.Attributes{}
		addrattr = addrattr.WithValue("dservers", info.DServers)
		serveraddrs = append(serveraddrs, gresolver.Address{
			Addr:       addr,
			Attributes: addrattr,
		})
	}
	s.Endpoints = append(s.Endpoints, gresolver.Endpoint{
		Addresses:  serveraddrs,
		Attributes: serverattr,
	})
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
		ww:      waitwake.NewWaitWake(),
		lker:    &sync.RWMutex{},
		servers: make(map[string]*ServerForPick),
		picker:  picker.NewPicker(nil),
	}
	cc.UpdateState(balancer.State{ConnectivityState: connectivity.Idle, Picker: b.c.balancer})
	return b.c.balancer
}

func (b *balancerBuilder) Name() string {
	return "corelib"
}

type corelibBalancer struct {
	c                *CGrpcClient
	cc               balancer.ClientConn
	ww               *waitwake.WaitWake
	lker             *sync.RWMutex
	servers          map[string]*ServerForPick
	picker           *picker.Picker
	lastResolveError error
}

// UpdateClientConnState and SubConn's StateListener are called sync by ccBalancerWrapper
func (b *corelibBalancer) UpdateClientConnState(ss balancer.ClientConnState) error {
	b.lker.Lock()
	b.lastResolveError = nil
	defer func() {
		if len(b.servers) == 0 {
			b.ww.Wake("CALL")
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Idle, Picker: b})
		} else if b.picker.ServerLen() > 0 {
			b.ww.Wake("CALL")
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Ready, Picker: b})
		} else {
			b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Connecting, Picker: b})
		}
		for addr := range b.servers {
			b.ww.Wake("SPECIFIC:" + addr)
		}
		b.lker.Unlock()
	}()
	//offline
	for _, server := range b.servers {
		find := false
		for _, addr := range ss.ResolverState.Endpoints[0].Addresses {
			if addr.Addr == server.addr {
				find = true
				break
			}
		}
		if !find {
			server.dservers = nil
			server.Pickinfo.SetDiscoverServerOffline(0)
		}
	}
	//online or update
	for _, v := range ss.ResolverState.Endpoints[0].Addresses {
		addr := v
		dservers, _ := addr.Attributes.Value("dservers").(map[string]*struct{})
		server, ok := b.servers[addr.Addr]
		if !ok {
			//this is a new register
			if len(dservers) == 0 {
				continue
			}
			sc, e := b.cc.NewSubConn([]gresolver.Address{addr}, balancer.NewSubConnOptions{
				HealthCheckEnabled: true,
				StateListener: func(s balancer.SubConnState) {
					b.lker.Lock()
					defer b.lker.Unlock()
					server, ok := b.servers[addr.Addr]
					if !ok {
						return
					}
					defer func() {
						if len(b.servers) == 0 {
							b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Idle, Picker: b})
						} else if b.picker.ServerLen() > 0 {
							b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Ready, Picker: b})
						} else {
							b.cc.UpdateState(balancer.State{ConnectivityState: connectivity.Connecting, Picker: b})
						}
					}()
					oldstatus := server.status
					server.status = int32(s.ConnectivityState)
					switch s.ConnectivityState {
					case connectivity.Shutdown:
						if oldstatus == int32(connectivity.Ready) {
							//offline
							log.Info(nil, "[cgrpc.client] offline", log.String("sname", b.c.server), log.String("sip", server.addr))
							go b.rebuildpicker(server.addr, false)
						}
						delete(b.servers, addr.Addr)
						b.ww.Wake("SPECIFIC:" + server.addr)
					case connectivity.Idle:
						if oldstatus == int32(connectivity.Ready) {
							//offline
							log.Info(nil, "[cgrpc.client] offline", log.String("sname", b.c.server), log.String("sip", server.addr))
							go b.rebuildpicker(server.addr, false)
						}
						if len(server.dservers) == 0 {
							server.status = int32(connectivity.Shutdown)
							delete(b.servers, addr.Addr)
							b.ww.Wake("SPECIFIC:" + server.addr)
							server.subconn.Shutdown()
						} else {
							//subconn's Connect is async inside
							server.subconn.Connect()
						}
					case connectivity.Ready:
						//online
						server.closing = 0
						log.Info(nil, "[cgrpc.client] online", log.String("sname", b.c.server), log.String("sip", server.addr))
						go b.rebuildpicker(server.addr, true)
					case connectivity.TransientFailure:
						//connect failed
						log.Error(nil, "[cgrpc.client] connect failed", log.String("sname", b.c.server), log.String("sip", server.addr), log.CError(s.ConnectionError))
					case connectivity.Connecting:
						log.Info(nil, "[cgrpc.client] connecting", log.String("sname", b.c.server), log.String("sip", server.addr))
					}
				},
			})
			if e != nil {
				//this can only happened on client is closing
				continue
			}
			server = &ServerForPick{
				addr:     addr.Addr,
				subconn:  sc,
				dservers: dservers,
				status:   int32(connectivity.Idle),
				Pickinfo: &picker.ServerPickInfo{},
			}
			server.Pickinfo.SetDiscoverServerOnline(uint32(len(dservers)))
			b.servers[addr.Addr] = server
			//subconn's Connect is async inside
			sc.Connect()
		} else if len(dservers) == 0 {
			//this is not a new register and this register is offline
			server.dservers = nil
			server.Pickinfo.SetDiscoverServerOffline(0)
		} else {
			//this is not a new register
			//unregister on which discovery server
			dserveroffline := false
			for dserver := range server.dservers {
				if _, ok := dservers[dserver]; !ok {
					dserveroffline = true
					break
				}
			}
			//register on which new discovery server
			for dserver := range dservers {
				if _, ok := server.dservers[dserver]; !ok {
					dserveroffline = false
					break
				}
			}
			server.dservers = dservers
			if dserveroffline {
				server.Pickinfo.SetDiscoverServerOffline(uint32(len(dservers)))
			} else {
				server.Pickinfo.SetDiscoverServerOnline(uint32(len(dservers)))
			}
		}
	}
	return nil
}

func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
	b.ww.Wake("CALL")
}

// Deprecated: replaced by StateListener in UpdateClientConnState's NewSubConn's options
func (b *corelibBalancer) UpdateSubConnState(_ balancer.SubConn, _ balancer.SubConnState) {
}

func (b *corelibBalancer) Close() {
	for _, server := range b.servers {
		server.subconn.Shutdown()
		log.Info(nil, "[cgrpc.client] offline", log.String("sname", b.c.server), log.String("sip", server.addr))
	}
	b.servers = make(map[string]*ServerForPick)
	b.lastResolveError = cerror.ErrClientClosing
	b.picker = picker.NewPicker(nil)
	b.ww.Wake("CALL")
}

// OnOff - true,online
// OnOff - false,offline
func (b *corelibBalancer) rebuildpicker(serveraddr string, OnOff bool) {
	b.lker.RLock()
	tmp := make([]picker.ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if server.Pickable() {
			tmp = append(tmp, server)
		}
	}
	b.lker.RUnlock()
	b.picker = picker.NewPicker(tmp)
	b.ww.Wake("SPECIFIC:" + serveraddr)
	if OnOff {
		//when online server,wake the block call
		b.ww.Wake("CALL")
	}
}

func (b *corelibBalancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	span := trace.SpanFromContext(info.Ctx)
	forceaddr, _ := info.Ctx.Value(forceaddrkey{}).(string)
	refresh := false
	for {
		server, done := b.picker.Pick(forceaddr)
		if server != nil {
			if dl, ok := info.Ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				//at least 5ms for net lag and server logic
				done(0, 0, false)
				return balancer.PickResult{}, cerror.ErrDeadlineExceeded
			}
			span.GetSelfSpanData().SetStateKV("host", server.GetServerAddr())
			return balancer.PickResult{
				SubConn: server.(*ServerForPick).subconn,
				Done: func(doneinfo balancer.DoneInfo) {
					e := transGrpcError(doneinfo.Err)
					cpuusagestrs := doneinfo.Trailer.Get("Cpu-Usage")
					var cpuusage float64
					if len(cpuusagestrs) > 0 {
						cpuusage, _ = strconv.ParseFloat(cpuusagestrs[0], 64)
					}
					span.Finish(e)
					done(cpuusage, uint64(span.GetEnd()-span.GetStart()), e == nil)
					monitor.GrpcClientMonitor(b.c.server, "GRPC", info.FullMethodName, e, uint64(span.GetEnd()-span.GetStart()))
					if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrTarget) {
						if atomic.SwapInt32(&server.(*ServerForPick).closing, 1) == 0 {
							//set the lowest pick priority
							server.(*ServerForPick).Pickinfo.SetDiscoverServerOffline(0)
							//rebuild picker
							b.rebuildpicker(server.(*ServerForPick).addr, false)
							//triger discover
							b.c.resolver.Now()
						}
					}
				},
			}, nil
		}
		if forceaddr == "" {
			if refresh {
				if b.lastResolveError != nil {
					return balancer.PickResult{}, b.lastResolveError
				}
				return balancer.PickResult{}, cerror.ErrNoserver
			}
			if e := b.ww.Wait(info.Ctx, "CALL", b.c.resolver.Now, nil); e != nil {
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
			continue
		}

		//maybe the forceaddr's server is connecting
		b.lker.RLock()
		s, ok := b.servers[forceaddr]
		if refresh && !ok {
			b.lker.RUnlock()
			if b.lastResolveError != nil {
				return balancer.PickResult{}, b.lastResolveError
			}
			return balancer.PickResult{}, cerror.ErrNoSpecificserver
		} else if ok {
			if s.closing == 1 {
				return balancer.PickResult{}, cerror.ErrNoSpecificserver
			}
			//server is connecting
			if e := b.ww.Wait(info.Ctx, "SPECIFIC:"+forceaddr, b.c.resolver.Now, b.lker.RUnlock); e != nil {
				return balancer.PickResult{}, cerror.ConvertStdError(e)
			}
		} else if !refresh {
			if e := b.ww.Wait(info.Ctx, "CALL", b.c.resolver.Now, b.lker.RUnlock); e != nil {
				return balancer.PickResult{}, cerror.ConvertStdError(e)
			}
			refresh = true
		}
	}
}
