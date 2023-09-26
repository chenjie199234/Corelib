package crpc

import (
	"context"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/internal/resolver"
)

type corelibBalancer struct {
	c                *CrpcClient
	lker             *sync.RWMutex
	version          discover.Version
	servers          map[string]*ServerForPick //key server addr
	picker           picker.PI
	lastResolveError error
}

func newCorelibBalancer(c *CrpcClient) *corelibBalancer {
	return &corelibBalancer{
		c:       c,
		lker:    &sync.RWMutex{},
		servers: make(map[string]*ServerForPick),
		picker:  picker.NewPicker(),
	}
}

func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
}

// version can only be int64 or string(should only be used with != or ==)
func (b *corelibBalancer) UpdateDiscovery(all map[string]*discover.RegisterData, version discover.Version) {
	b.lastResolveError = nil
	b.lker.Lock()
	defer func() {
		if len(b.servers) == 0 || b.picker.ServerLen() > 0 {
			b.c.resolver.Wake(resolver.CALL)
		}
		b.c.resolver.Wake(resolver.SYSTEM)
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
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().UnixNano()
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
				Pickinfo: &picker.ServerPickInfo{
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
			}
			b.servers[addr] = server
			go b.c.start(server, false)
		} else if registerdata == nil || len(registerdata.DServers) == 0 {
			//this is not a new register and this register is offline
			server.dservers = nil
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().UnixNano()
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range server.dservers {
				if _, ok := registerdata.DServers[dserver]; !ok {
					server.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range registerdata.DServers {
				if _, ok := server.dservers[dserver]; !ok {
					server.Pickinfo.DServerOffline = 0
					break
				}
			}
			server.dservers = registerdata.DServers
			server.Pickinfo.Addition = registerdata.Addition
			server.Pickinfo.DServerNum = int32(len(registerdata.DServers))
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
		return false
	}
	b.lker.Unlock()
	time.Sleep(time.Millisecond * 100)
	//need to check server register status
	b.c.resolver.Wait(context.Background(), resolver.SYSTEM)
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
func (b *corelibBalancer) RebuildPicker(OnOff bool) {
	b.lker.Lock()
	tmp := make([]picker.ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if server.Pickable() {
			tmp = append(tmp, server)
		}
	}
	b.picker.UpdateServers(tmp)
	if OnOff {
		//when online server,wake the block call
		b.c.resolver.Wake(resolver.CALL)
	}
	b.lker.Unlock()
}
func (b *corelibBalancer) Pick(ctx context.Context, forceaddr string) (server *ServerForPick, done func(), e error) {
	refresh := false
	for {
		server, done := b.picker.Pick(forceaddr)
		if server != nil {
			if dl, ok := ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				//at least 5ms for net lag and server logic
				done()
				return nil, nil, cerror.ErrDeadlineExceeded
			}
			return server.(*ServerForPick), done, nil
		}
		if refresh {
			if b.lastResolveError != nil {
				return nil, done, b.lastResolveError
			}
			if forceaddr != "" {
				return nil, nil, cerror.ErrNoSpecificserver
			}
			return nil, nil, cerror.ErrNoserver
		}
		if e := b.c.resolver.Wait(ctx, resolver.CALL); e != nil {
			if e == context.DeadlineExceeded {
				return nil, nil, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return nil, nil, cerror.ErrCanceled
			} else {
				//this is impossible
				return nil, nil, cerror.ConvertStdError(e)
			}
		}
		refresh = true
	}
}
