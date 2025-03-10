package web

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/util/waitwake"
)

type corelibBalancer struct {
	c                *WebClient
	ww               *waitwake.WaitWake
	lker             *sync.RWMutex
	version          discover.Version
	servers          map[string]*ServerForPick //key server addr
	picker           *picker.Picker
	lastResolveError error
}

func newCorelibBalancer(c *WebClient) *corelibBalancer {
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
}

// version can only be int64 or string(should only be used with != or ==)
func (b *corelibBalancer) UpdateDiscovery(all map[string]*discover.RegisterData, version discover.Version) {
	b.lastResolveError = nil
	b.lker.Lock()
	defer func() {
		for _, server := range b.servers {
			server.closing.Store(false)
		}
		b.rebuildpicker()
		b.ww.Wake("CALL")
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
			delete(b.servers, server.addr)
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
				addr:     addr,
				dservers: registerdata.DServers,
				Pickinfo: picker.NewServerPickInfo(),
			}
			server.Pickinfo.SetDiscoverServerOnline(uint32(len(registerdata.DServers)))
			b.servers[addr] = server
		} else if registerdata == nil || len(registerdata.DServers) == 0 {
			//this is not a new register and this register is offline
			delete(b.servers, addr)
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
			server.closing.Store(false)
		}
	}
}
func (b *corelibBalancer) rebuildpicker() {
	tmp := make([]picker.ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if server.Pickable() {
			tmp = append(tmp, server)
		}
	}
	newpicker := picker.NewPicker(tmp)
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&b.picker)), unsafe.Pointer(newpicker))
	return
}
func (b *corelibBalancer) Pick(ctx context.Context) (*ServerForPick, error) {
	forceaddr, _ := ctx.Value(forceaddrkey{}).(string)
	refresh := false
	for {
		server := (*picker.Picker)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&b.picker)))).Pick(forceaddr)
		if server != nil {
			if dl, ok := ctx.Deadline(); ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				//at least 5ms for net lag and server logic
				server.GetServerPickInfo().Done(false)
				return nil, cerror.ErrDeadlineExceeded
			}
			return server.(*ServerForPick), nil
		}
		if refresh {
			if b.lastResolveError != nil {
				return nil, b.lastResolveError
			}
			if forceaddr != "" {
				return nil, cerror.ErrNoSpecificserver
			}
			return nil, cerror.ErrNoserver
		}
		if e := b.ww.Wait(ctx, "CALL", b.c.resolver.Now, nil); e != nil {
			if e == context.DeadlineExceeded {
				return nil, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return nil, cerror.ErrCanceled
			} else {
				//this is impossible
				return nil, cerror.Convert(e)
			}
		}
		refresh = true
	}
}
