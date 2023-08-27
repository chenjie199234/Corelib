package web

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/internal/resolver"
)

type corelibBalancer struct {
	c                *WebClient
	lker             *sync.RWMutex
	serversRaw       []byte
	servers          map[string]*ServerForPick //key server addr
	picker           picker.PI
	lastResolveError error
}
type ServerForPick struct {
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}

func newCorelibBalancer(c *WebClient) *corelibBalancer {
	return &corelibBalancer{
		c:          c,
		lker:       &sync.RWMutex{},
		serversRaw: nil,
		servers:    make(map[string]*ServerForPick),
		picker:     picker.NewPicker(),
	}
}
func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
}
func (b *corelibBalancer) UpdateDiscovery(all map[string]*discover.RegisterData) {
	b.lastResolveError = nil
	d, _ := json.Marshal(all)
	b.lker.Lock()
	defer func() {
		b.rebuildpicker()

		b.c.resolver.Wake(resolver.CALL)
		b.c.resolver.Wake(resolver.SYSTEM)
		b.lker.Unlock()
	}()
	if bytes.Equal(b.serversRaw, d) {
		return
	}
	b.serversRaw = d
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
				Pickinfo: &picker.ServerPickInfo{
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
			}
			b.servers[addr] = server
		} else if registerdata == nil || len(registerdata.DServers) == 0 {
			//this is not a new register and this register is offline
			delete(b.servers, addr)
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
func (b *corelibBalancer) rebuildpicker() {
	tmp := make([]picker.ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		tmp = append(tmp, server)
	}
	b.picker.UpdateServers(tmp)
	return
}
func (b *corelibBalancer) Pick(ctx context.Context) (*ServerForPick, func(), error) {
	refresh := false
	for {
		server, done := b.picker.Pick()
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
				return nil, nil, b.lastResolveError
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
