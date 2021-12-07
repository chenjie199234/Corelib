package cgrpc

import (
	"context"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

type balancerBuilder struct {
	c *CGrpcClient
}

func (b *balancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	b.c.balancer = &corelibBalancer{
		c:       b.c,
		cc:      cc,
		servers: make(map[balancer.SubConn]*ServerForPick),
	}
	cc.UpdateState(balancer.State{ConnectivityState: connectivity.Ready, Picker: b.c.balancer})
	return b.c.balancer
}

func (b *balancerBuilder) Name() string {
	return "corelib"
}

type corelibBalancer struct {
	c        *CGrpcClient
	cc       balancer.ClientConn
	servers  map[balancer.SubConn]*ServerForPick
	pservers []*ServerForPick
}

type ServerForPick struct {
	addr     string
	subconn  balancer.SubConn
	dservers map[string]struct{} //this app registered on which discovery server
	status   int32

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
	return atomic.LoadInt32(&s.status) == int32(connectivity.Ready)
}

func (b *corelibBalancer) setPickerServers(servers []*ServerForPick) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&b.pservers)), unsafe.Pointer(&servers))
}
func (b *corelibBalancer) getPickServers() []*ServerForPick {
	tmp := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&b.pservers)))
	if tmp == nil {
		return nil
	}
	return *(*[]*ServerForPick)(tmp)
}

func (b *corelibBalancer) UpdateClientConnState(ss balancer.ClientConnState) error {
	defer func() {
		if len(b.pservers) > 0 || len(b.servers) == 0 {
			b.c.resolver.wakemanual()
		}
	}()
	//offline
	for _, exist := range b.servers {
		find := false
		for _, addr := range ss.ResolverState.Addresses {
			if addr.Addr == exist.addr {
				find = true
				break
			}
		}
		if !find {
			exist.dservers = nil
			exist.Pickinfo.DServerNum = 0
			exist.Pickinfo.DServerOffline = time.Now().UnixNano()
		}
	}
	//online or update
	for _, addr := range ss.ResolverState.Addresses {
		dservers, _ := addr.BalancerAttributes.Value("dservers").(map[string]struct{})
		addition, _ := addr.BalancerAttributes.Value("addition").([]byte)
		var exist *ServerForPick
		for _, v := range b.servers {
			if v.addr == addr.Addr {
				exist = v
				break
			}
		}
		if exist == nil {
			//this is a new register
			sc, e := b.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{HealthCheckEnabled: true})
			if e != nil {
				//this can only happened on client is closing
				//but in corelib this will not be closed
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
			go sc.Connect()
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range exist.dservers {
				if _, ok := dservers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range dservers {
				if _, ok := exist.dservers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = 0
					break
				}
			}
			exist.dservers = dservers
			exist.Pickinfo.Addition = addition
			exist.Pickinfo.DServerNum = int32(len(dservers))
		}
	}
	return nil
}

func (b *corelibBalancer) ResolverError(error) {

}

func (b *corelibBalancer) UpdateSubConnState(sc balancer.SubConn, s balancer.SubConnState) {
	exist, ok := b.servers[sc]
	if !ok {
		return
	}
	if s.ConnectivityState == connectivity.Shutdown {
		olds := atomic.LoadInt32(&exist.status)
		atomic.StoreInt32(&exist.status, int32(connectivity.Shutdown))
		delete(b.servers, sc)
		if olds == int32(connectivity.Ready) {
			b.rebuildpicker()
		}
		return
	}
	if s.ConnectivityState == connectivity.Idle && atomic.LoadInt32(&exist.status) == int32(connectivity.Ready) {
		log.Info(nil, "[cgrpc.client] server:", b.c.appname+":"+exist.addr, "offline")
	} else if s.ConnectivityState == connectivity.Ready {
		b.c.resolver.wakemanual()
		log.Info(nil, "[cgrpc.client] server:", b.c.appname+":"+exist.addr, "online")
	} else if s.ConnectivityState == connectivity.TransientFailure {
		log.Error(nil, "[cgrpc.client] connect to server:", b.c.appname+":"+exist.addr, "error:", s.ConnectionError)
	}
	olds := atomic.LoadInt32(&exist.status)
	atomic.StoreInt32(&exist.status, int32(s.ConnectivityState))
	if (olds == int32(connectivity.Ready) && s.ConnectivityState == connectivity.Idle) || s.ConnectivityState == connectivity.Ready {
		b.rebuildpicker()
	}
	if s.ConnectivityState == connectivity.Idle {
		if len(exist.dservers) == 0 {
			atomic.StoreInt32(&exist.status, int32(connectivity.Shutdown))
			delete(b.servers, sc)
			b.cc.RemoveSubConn(sc)
		} else {
			go sc.Connect()
		}
	}
}
func (b *corelibBalancer) rebuildpicker() {
	tmp := make([]*ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if atomic.LoadInt32(&server.status) == int32(connectivity.Ready) {
			tmp = append(tmp, server)
		}
	}
	b.setPickerServers(tmp)
	return
}

func (b *corelibBalancer) Close() {
}

func (b *corelibBalancer) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	refresh := false
	for {
		server := b.c.c.Picker(b.getPickServers())
		if server != nil {
			atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
			return balancer.PickResult{
				SubConn: server.subconn,
				Done: func(doneinfo balancer.DoneInfo) {
					atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
					if doneinfo.Err != nil {
						server.Pickinfo.LastFailTime = time.Now().UnixNano()
						if cerror.Equal(transGrpcError(doneinfo.Err), errClosing) {
							atomic.StoreInt32(&server.status, int32(connectivity.Shutdown))
						}
					}
				},
			}, nil
		}
		if refresh {
			return balancer.PickResult{}, ErrNoserver
		}
		if e := b.c.resolver.waitmanual(info.Ctx); e != nil {
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
