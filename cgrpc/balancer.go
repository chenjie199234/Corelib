package cgrpc

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/waitwake"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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
	}
	b.c.balancer.picker.Store(picker.NewPicker(nil))
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
	picker           atomic.Pointer[picker.Picker]
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
		} else if b.picker.Load().ServerLen() > 0 {
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
		dservers, _ := addr.Attributes.Value("dservers").(map[string]struct{})
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
						} else if b.picker.Load().ServerLen() > 0 {
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
							slog.Info("[cgrpc.client] offline", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr))
							go b.rebuildpicker(server.addr, false)
						}
						delete(b.servers, addr.Addr)
						b.ww.Wake("SPECIFIC:" + server.addr)
					case connectivity.Idle:
						if oldstatus == int32(connectivity.Ready) {
							//offline
							slog.Info("[cgrpc.client] offline", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr))
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
						server.closing.Store(false)
						slog.Info("[cgrpc.client] online", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr))
						go b.rebuildpicker(server.addr, true)
					case connectivity.TransientFailure:
						//connect failed
						slog.Error("[cgrpc.client] connect failed", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr), slog.String("error", s.ConnectionError.Error()))
					case connectivity.Connecting:
						slog.Info("[cgrpc.client] connecting", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr))
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
				Pickinfo: picker.NewServerPickInfo(),
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
	b.lker.Lock()
	b.lastResolveError = e
	b.lker.Unlock()
	b.ww.Wake("CALL")
}

// Deprecated: replaced by StateListener in UpdateClientConnState's NewSubConn's options
func (b *corelibBalancer) UpdateSubConnState(_ balancer.SubConn, _ balancer.SubConnState) {
}

func (b *corelibBalancer) Close() {
	for _, server := range b.servers {
		server.subconn.Shutdown()
		slog.Info("[cgrpc.client] offline", slog.String("sname", b.c.serverfullname), slog.String("sip", server.addr))
	}
	b.servers = make(map[string]*ServerForPick)
	b.lker.Lock()
	b.lastResolveError = cerror.ErrClientClosing
	b.lker.Unlock()
	b.picker.Store(picker.NewPicker(nil))
	b.ww.Wake("CALL")
}

func (b *corelibBalancer) ExitIdle() {
	// we always keep connection,there will not exist idle status,so we don't need this function
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
	b.picker.Store(picker.NewPicker(tmp))
	b.ww.Wake("SPECIFIC:" + serveraddr)
	if OnOff {
		//when online server,wake the block call
		b.ww.Wake("CALL")
	}
}

func (b *corelibBalancer) Pick(info balancer.PickInfo) (pickinfo balancer.PickResult, e error) {
	if err := b.c.stop.Add(1); e != nil {
		if err == graceful.ErrClosing {
			e = cerror.ErrClientClosing
		} else {
			e = cerror.ErrBusy
		}
		return
	}
	defer func() {
		if pickinfo.SubConn == nil || pickinfo.Done == nil {
			b.c.stop.DoneOne()
		}
	}()
	span := trace.SpanFromContext(info.Ctx)
	stime := info.Ctx.Value(stimekey{}).(int64)
	forceaddr, _ := info.Ctx.Value(forceaddrkey{}).(string)
	refresh := false
	for {
		server := b.picker.Load().Pick(forceaddr)
		if server != nil {
			if dl, ok := info.Ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(time.Millisecond) {
				//at least 1ms for net lag and server logic
				e = cerror.ErrDeadlineExceeded
				return
			}
			pickinfo.SubConn = server.(*ServerForPick).subconn
			pickinfo.Done = func(doneinfo balancer.DoneInfo) {
				e := transGrpcError(doneinfo.Err, false)
				span.SetAttributes(attribute.String("sip", server.(*ServerForPick).addr))
				if e != nil {
					span.SetStatus(codes.Error, e.Error())
				} else {
					span.SetStatus(codes.Ok, "")
				}
				span.End()
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				server.GetServerPickInfo().Done(e == nil, uint64(etime-stime))
				if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrTarget) {
					//server will not handle this call,we can retry this request
					//the retry will happened in Client's Invoke or NewStream function
					if !server.(*ServerForPick).closing.Swap(true) {
						//set the lowest pick priority
						server.(*ServerForPick).Pickinfo.SetDiscoverServerOffline(0)
						//rebuild picker
						b.rebuildpicker(server.(*ServerForPick).addr, false)
						//triger discover
						b.c.resolver.Now()
					}
				} else if cotel.NeedMetric() {
					//only record the real call's metric
					b.c.recordmetric(info.FullMethodName, float64(etime-stime)/1000000.0, e != nil)
				}
				b.c.stop.DoneOne()
			}
			return
		}
		if forceaddr == "" {
			if refresh {
				b.lker.RLock()
				e = b.lastResolveError
				b.lker.RUnlock()
				if e == nil {
					e = cerror.ErrNoserver
				}
				return
			}
			if err := b.ww.Wait(info.Ctx, "CALL", b.c.resolver.Now, nil); e != nil {
				switch err {
				case context.DeadlineExceeded:
					e = cerror.ErrDeadlineExceeded
					return
				case context.Canceled:
					e = cerror.ErrCanceled
					return
				default:
					//this is impossible
					e = cerror.Convert(e)
					return
				}
			}
			refresh = true
			continue
		}

		//maybe the forceaddr's server is connecting
		b.lker.RLock()
		s, ok := b.servers[forceaddr]
		if !ok { //the specific server not exist
			if refresh {
				e = b.lastResolveError
				b.lker.RUnlock()
				if e == nil {
					e = cerror.ErrNoSpecificserver
				}
				return
			} else if err := b.ww.Wait(info.Ctx, "CALL", b.c.resolver.Now, b.lker.RUnlock); err != nil { //wait the discover to refresh the server info
				e = cerror.Convert(e)
				return
			} else {
				refresh = true
			}
		} else if s.closing.Load() { //the specific server exist but it is closing
			b.lker.RUnlock()
			e = cerror.ErrNoSpecificserver
			return
		} else if err := b.ww.Wait(info.Ctx, "SPECIFIC:"+forceaddr, b.c.resolver.Now, b.lker.RUnlock); err != nil { //the specific server exist but is connecting,we need to wait
			e = cerror.Convert(e)
			return
		}
	}
}
