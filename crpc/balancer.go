package crpc

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/stream"
	"google.golang.org/protobuf/proto"
)

type corelibBalancer struct {
	c                *CrpcClient
	lker             *sync.RWMutex
	serversRaw       []byte
	servers          map[string]*ServerForPick //key server addr
	lastResolveError error
}
type ServerForPick struct {
	callid   uint64 //start from 100
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server
	peer     *stream.Peer
	closing  bool

	//active calls
	lker *sync.Mutex
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}

func (s *ServerForPick) getpeer() *stream.Peer {
	return (*stream.Peer)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer))))
}
func (s *ServerForPick) setpeer(p *stream.Peer) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(p))
}
func (s *ServerForPick) caspeer(oldpeer, newpeer *stream.Peer) bool {
	return atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(&oldpeer), unsafe.Pointer(newpeer))
}
func (s *ServerForPick) Pickable() bool {
	return s.getpeer() != nil && !s.closing
}

func (s *ServerForPick) sendmessage(ctx context.Context, r *req) (e error) {
	p := s.getpeer()
	if p == nil {
		return cerror.ErrClosed
	}
	beforeSend := func(_ *stream.Peer) {
		s.lker.Lock()
	}
	afterSend := func(_ *stream.Peer, e error) {
		if e != nil {
			s.lker.Unlock()
			return
		}
		s.reqs[r.reqdata.Callid] = r
		s.lker.Unlock()
	}
	d, _ := proto.Marshal(r.reqdata)
	if e = p.SendMessage(ctx, d, beforeSend, afterSend); e != nil {
		if e == stream.ErrMsgLarge {
			e = cerror.ErrReqmsgLen
		} else if e == stream.ErrConnClosed {
			e = cerror.ErrClosed
			s.caspeer(p, nil)
		} else if e == context.DeadlineExceeded {
			e = cerror.ErrDeadlineExceeded
		} else if e == context.Canceled {
			e = cerror.ErrCanceled
		} else {
			//this is impossible
			e = cerror.ConvertStdError(e)
		}
		return
	}
	return
}
func (s *ServerForPick) sendcancel(ctx context.Context, canceldata []byte) {
	if p := s.getpeer(); p != nil {
		p.SendMessage(ctx, canceldata, nil, nil)
	}
}
func newCorelibBalancer(c *CrpcClient) *corelibBalancer {
	return &corelibBalancer{
		c:          c,
		lker:       &sync.RWMutex{},
		serversRaw: nil,
		servers:    make(map[string]*ServerForPick),
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
		if len(b.servers) == 0 || b.c.picker.ServerLen() > 0 {
			b.c.resolver.Wake(resolver.CALL)
		}
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
	b.c.picker.UpdateServers(tmp)
	if OnOff {
		//when online server,wake the block call
		b.c.resolver.Wake(resolver.CALL)
	}
	b.lker.Unlock()
}
func (b *corelibBalancer) Pick(ctx context.Context) (server *ServerForPick, done func(), e error) {
	refresh := false
	for {
		server, done := b.c.picker.Pick()
		if server != nil {
			return server.(*ServerForPick), done, nil
		}
		if refresh {
			if b.lastResolveError != nil {
				return nil, done, b.lastResolveError
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
