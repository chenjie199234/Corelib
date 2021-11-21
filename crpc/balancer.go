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
)

type corelibBalancer struct {
	c          *CrpcClient
	lker       *sync.RWMutex
	serversRaw []byte
	servers    map[string]*ServerForPick //key server addr
}
type ServerForPick struct {
	addr     string
	dservers map[string]struct{} //this app registered on which discovery server
	peer     *stream.Peer
	status   int32 //2 - working,1 - connecting,0 - closed

	//active calls
	lker *sync.Mutex
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nanosecond
	Activecalls    int32  //current active calls
	DServerNum     int32  //this app registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

func (s *ServerForPick) Pickable() bool {
	return atomic.LoadInt32(&s.status) == 2
}

func (s *ServerForPick) sendmessage(ctx context.Context, r *req) (e error) {
	p := (*stream.Peer)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer))))
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
		s.reqs[r.callid] = r
		s.lker.Unlock()
	}
	if e = p.SendMessage(ctx, r.req, beforeSend, afterSend); e != nil {
		if e == stream.ErrMsgLarge {
			e = ErrReqmsgLen
		} else if e == stream.ErrConnClosed {
			e = errPickAgain
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
	if p := (*stream.Peer)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)))); p != nil {
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
func (b *corelibBalancer) UpdateDiscovery(all map[string]*RegisterData) {
	d, _ := json.Marshal(all)
	b.lker.Lock()
	defer func() {
		if len(b.servers) == 0 {
			b.c.resolver.wakemanual()
		} else {
			for _, server := range b.servers {
				if server.Pickable() {
					b.c.resolver.wakemanual()
					break
				}
			}
		}
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
			server := &ServerForPick{
				addr:     addr,
				dservers: registerdata.DServers,
				peer:     nil,
				status:   1,
				lker:     &sync.Mutex{},
				reqs:     make(map[uint64]*req, 10),
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
			}
			b.servers[addr] = server
			go b.c.start(server)
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
func (b *corelibBalancer) GetRegisterServer(addr string) *ServerForPick {
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
	//need to check server register status
	b.c.resolver.waitmanual(context.Background())
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
func (b *corelibBalancer) Pick(ctx context.Context) (*ServerForPick, error) {
	refresh := false
	for {
		b.lker.RLock()
		server := b.c.c.Picker(b.servers)
		if server != nil {
			b.lker.RUnlock()
			return server, nil
		}
		if refresh {
			b.lker.RUnlock()
			return nil, ErrNoserver
		}
		b.lker.RUnlock()
		if e := b.c.resolver.waitmanual(ctx); e != nil {
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
