package crpc

import (
	"context"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/util/waitwake"
)

type corelibBalancer struct {
	c                *CrpcClient
	ww               *waitwake.WaitWake
	lker             *sync.RWMutex
	version          discover.Version
	servers          map[string]*ServerForPick //key server addr
	picker           *picker.Picker
	lastResolveError error
}

func newCorelibBalancer(c *CrpcClient) *corelibBalancer {
	return &corelibBalancer{
		c:       c,
		ww:      waitwake.NewWaitWake(),
		lker:    &sync.RWMutex{},
		servers: make(map[string]*ServerForPick),
		picker:  picker.NewPicker(nil),
	}
}

func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
	b.ww.Wake("CALL")
	//this is only for ReconnectCheck
	b.ww.Wake("SYSTEM")
}

// version can only be int64 or string(should only be used with != or ==)
func (b *corelibBalancer) UpdateDiscovery(all map[string]*discover.RegisterData, version discover.Version) {
	b.lastResolveError = nil
	b.lker.Lock()
	defer func() {
		if len(b.servers) == 0 || b.picker.ServerLen() > 0 {
			b.ww.Wake("CALL")
		}
		//this is only for ReconnectCheck
		b.ww.Wake("SYSTEM")
		for addr := range b.servers {
			b.ww.Wake("SPECIFIC:" + addr)
		}
		b.lker.Unlock()
	}()
	if discover.SameVersion(b.version, version) {
		return
	}
	b.version = version
	//offline app
	for _, server := range b.servers {
		if _, ok := all[server.addr]; !ok {
			//this app unregistered
			server.dservers = nil
			server.Pickinfo.SetDiscoverServerOffline(0)
		}
	}
	//online app or update app's dservers
	for addr, registerdata := range all {
		server, ok := b.servers[addr]
		if !ok {
			//this is a new register
			if registerdata == nil || len(registerdata.DServers) == 0 {
				continue
			}
			server := &ServerForPick{
				callid:   100, //start from 100
				addr:     addr,
				dservers: registerdata.DServers,
				peer:     nil,
				lker:     &sync.Mutex{},
				reqs:     make(map[uint64]*req, 10),
				Pickinfo: &picker.ServerPickInfo{},
			}
			server.Pickinfo.SetDiscoverServerOnline(uint32(len(registerdata.DServers)))
			b.servers[addr] = server
			go b.c.start(server, false)
		} else if registerdata == nil || len(registerdata.DServers) == 0 {
			//this is not a new register and this register is offline
			server.dservers = nil
			server.Pickinfo.SetDiscoverServerOffline(0)
		} else {
			//this is not a new register
			//unregister on which discovery server
			dserveroffline := false
			for dserver := range server.dservers {
				if _, ok := registerdata.DServers[dserver]; !ok {
					dserveroffline = true
					break
				}
			}
			//register on which new discovery server
			for dserver := range registerdata.DServers {
				if _, ok := server.dservers[dserver]; !ok {
					dserveroffline = false
					break
				}
			}
			server.dservers = registerdata.DServers
			if dserveroffline {
				server.Pickinfo.SetDiscoverServerOffline(uint32(len(registerdata.DServers)))
			} else {
				server.Pickinfo.SetDiscoverServerOnline(uint32(len(registerdata.DServers)))
			}
		}
	}
}
func (b *corelibBalancer) getRegisterServer(addr string) *ServerForPick {
	b.lker.RLock()
	server, ok := b.servers[addr]
	if !ok {
		b.lker.RUnlock()
		return nil
	}
	if len(server.dservers) == 0 {
		//server already unregister
		b.lker.RUnlock()
		return nil
	}
	b.lker.RUnlock()
	return server
}
func (b *corelibBalancer) ReconnectCheck(server *ServerForPick) bool {
	b.lker.Lock()
	if len(server.dservers) == 0 {
		//server already unregister,remove server
		delete(b.servers, server.addr)
		b.lker.Unlock()
		b.ww.Wake("SPECIFIC:" + server.addr)
		return false
	}
	b.lker.Unlock()
	time.Sleep(time.Millisecond * 100)
	//need to check server register status
	b.ww.Wait(context.Background(), "SYSTEM", b.c.resolver.Now, nil)
	b.lker.Lock()
	if len(server.dservers) == 0 {
		//server already unregister,remove server
		delete(b.servers, server.addr)
		b.lker.Unlock()
		return false
	}
	b.lker.Unlock()
	return true
}

// OnOff - true,online
// OnOff - false,offline
func (b *corelibBalancer) RebuildPicker(serveraddr string, OnOff bool) {
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
func (b *corelibBalancer) Pick(ctx context.Context) (server *ServerForPick, done func(cpuusage float64, successwastetime uint64, success bool), e error) {
	forceaddr, _ := ctx.Value(forceaddrkey{}).(string)
	refresh := false
	for {
		server, done := b.picker.Pick(forceaddr)
		if server != nil {
			if dl, ok := ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				//at least 5ms for net lag and server logic
				done(0, 0, false)
				return nil, nil, cerror.ErrDeadlineExceeded
			}
			return server.(*ServerForPick), done, nil
		}
		if forceaddr == "" {
			if refresh {
				if b.lastResolveError != nil {
					return nil, done, b.lastResolveError
				}
				return nil, nil, cerror.ErrNoserver
			}
			if e := b.ww.Wait(ctx, "CALL", b.c.resolver.Now, nil); e != nil {
				return nil, nil, cerror.Convert(e)
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
				return nil, done, b.lastResolveError
			}
			return nil, nil, cerror.ErrNoSpecificserver
		} else if ok {
			if s.closing == 1 {
				return nil, nil, cerror.ErrNoSpecificserver
			}
			//server is connecting
			if e := b.ww.Wait(ctx, "SPECIFIC:"+forceaddr, b.c.resolver.Now, b.lker.RUnlock); e != nil {
				return nil, nil, cerror.Convert(e)
			}
		} else if !refresh {
			if e := b.ww.Wait(ctx, "CALL", b.c.resolver.Now, b.lker.RUnlock); e != nil {
				return nil, nil, cerror.Convert(e)
			}
			refresh = true
		}
	}
}
