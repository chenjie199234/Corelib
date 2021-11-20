package grpc

import (
	"sync/atomic"
	"time"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

type balancerBuilder struct {
	c *GrpcClient
}

// Build creates a new balancer with the ClientConn.
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
	c       *GrpcClient
	cc      balancer.ClientConn
	servers map[balancer.SubConn]*ServerForPick
	picker  *corelibPicker
}

type ServerForPick struct {
	addr     string
	subconn  balancer.SubConn
	dservers map[string]struct{} //this app registered on which discovery server
	status   connectivity.State

	Pickinfo *pickinfo
}

type pickinfo struct {
	Lastfail       int64  //last fail timestamp nano second
	Activecalls    int32  //current active calls
	DServerNum     int32  //this server registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

func (s *ServerForPick) Pickable() bool {
	return s.status == connectivity.Ready
}

func (b *corelibBalancer) UpdateClientConnState(ss balancer.ClientConnState) error {
	//offline
	for sc, exist := range b.servers {
		find := false
		for _, addr := range ss.ResolverState.Addresses {
			if addr.Addr == exist.addr {
				find = true
				break
			}
		}
		if !find {
			b.cc.RemoveSubConn(sc)
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
				status:   connectivity.Idle,
				Pickinfo: &pickinfo{
					Lastfail:       0,
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
		exist.status = connectivity.Shutdown
		delete(b.servers, sc)
		b.rebuildpicker()
		return
	}
	if len(exist.dservers) == 0 {
		exist.status = connectivity.Shutdown
		delete(b.servers, sc)
		b.cc.RemoveSubConn(sc)
		b.rebuildpicker()
		return
	}
	exist.status = s.ConnectivityState
	if exist.status == connectivity.Idle || exist.status == connectivity.Ready {
		b.rebuildpicker()
	}
	if s.ConnectivityState == connectivity.Idle {
		go sc.Connect()
	}
}
func (b *corelibBalancer) rebuildpicker() balancer.Picker {
	servers := make(map[string]*ServerForPick, len(b.servers))
	for _, server := range b.servers {
		if server.status == connectivity.Ready {
			servers[server.addr] = server
		}
	}
	return &corelibPicker{c: b.c, servers: servers}
}

func (b *corelibBalancer) Close() {
}

type corelibPicker struct {
	c       *GrpcClient
	servers map[string]*ServerForPick
}

func (p *corelibPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	server := p.c.c.Picker(p.servers)
	if server == nil {
		return balancer.PickResult{}, ErrNoserver
	}
	atomic.AddInt32(&(server.Pickinfo.Activecalls), 1)
	return balancer.PickResult{
		SubConn: server.subconn,
		Done: func(doneinfo balancer.DoneInfo) {
			atomic.AddInt32(&(server.Pickinfo.Activecalls), -1)
			if doneinfo.Err != nil {
				server.Pickinfo.Lastfail = time.Now().UnixNano()
			}
		},
	}, nil
}

//type pickerbuilder struct {
//        c *GrpcClient
//}

//func (b *pickerbuilder) Build(info base.PickerBuildInfo) balancer.Picker {
//        servers := make(map[string]*ServerForPick)
//        for subconn, subconninfo := range info.ReadySCs {
//                dservers, _ := subconninfo.Address.BalancerAttributes.Value("dservers").(map[string]struct{})
//                addition, _ := subconninfo.Address.BalancerAttributes.Value("addition").([]byte)
//                servers[subconninfo.Address.Addr] = &ServerForPick{
//                        subconn:  subconn,
//                        addr:     subconninfo.Address.Addr,
//                        dservers: dservers,
//                        Pickinfo: &pickinfo{
//                                Lastfail:    0,
//                                Activecalls: 0,
//                                DServerNum:  int32(len(dservers)),
//                                Addition:    addition,
//                        },
//                }
//        }
//        return &corelibpicker{c: b.c}
//}

//type corelibpicker struct {
//        c *GrpcClient
//}

//func (p *corelibpicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
//        p.c.lker.RLock()
//        defer p.c.lker.RUnlock()
//        p.c.c.Picker(p.c.servers)
//}
