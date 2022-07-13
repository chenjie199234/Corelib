package crpc

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/stream"
	"google.golang.org/protobuf/proto"
)

type corelibBalancer struct {
	c                *CrpcClient
	lker             *sync.RWMutex
	serversRaw       []byte
	servers          map[string]*ServerForPick //key server addr
	pservers         []*ServerForPick
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

	Pickinfo *pickinfo
}
type pickinfo struct {
	LastFailTime   int64  //last fail timestamp nanosecond
	Activecalls    int32  //current active calls
	DServerNum     int32  //this app registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
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
		return errPickAgain
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
			e = errPickAgain
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
func (b *corelibBalancer) ResolverError(e error) {
	b.lastResolveError = e
}
func (b *corelibBalancer) UpdateDiscovery(all map[string]*RegisterData) {
	b.lastResolveError = nil
	d, _ := json.Marshal(all)
	b.lker.Lock()
	defer func() {
		if len(b.servers) == 0 || len(b.pservers) > 0 {
			b.c.resolver.wake(false)
		}
		b.c.resolver.wake(true)
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
				Pickinfo: &pickinfo{
					LastFailTime:   0,
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
	b.c.resolver.wait(context.Background(), true)
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

//reason - true,online
//reason - false,offline
func (b *corelibBalancer) RebuildPicker(reason bool) {
	b.lker.Lock()
	tmp := make([]*ServerForPick, 0, len(b.servers))
	for _, server := range b.servers {
		if server.Pickable() {
			tmp = append(tmp, server)
		}
	}
	b.setPickerServers(tmp)
	if reason {
		b.c.resolver.wake(false)
	}
	b.lker.Unlock()
}
func (b *corelibBalancer) Pick(ctx context.Context) (*ServerForPick, error) {
	refresh := false
	for {
		server := b.c.c.Picker(b.getPickServers())
		if server != nil {
			return server, nil
		}
		if refresh {
			if b.lastResolveError != nil {
				return nil, b.lastResolveError
			}
			return nil, cerror.ErrNoserver
		}
		if e := b.c.resolver.wait(ctx, false); e != nil {
			if e == context.DeadlineExceeded {
				return nil, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return nil, cerror.ErrCanceled
			} else {
				//this is impossible
				return nil, cerror.ConvertStdError(e)
			}
		}
		refresh = true
	}
}
