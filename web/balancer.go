package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
)

type corelibBalancer struct {
	c          *WebClient
	lker       *sync.RWMutex
	serversRaw []byte
	servers    map[string]*ServerForPick //key server addr
}
type ServerForPick struct {
	addr     string
	client   *http.Client
	dservers map[string]struct{} //this server registered on how many discoveryservers
	status   int32               //1 - working,0 - closed

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
	return atomic.LoadInt32(&s.status) == 1
}
func newCorelibBalancer(c *WebClient) *corelibBalancer {
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
		b.lker.Unlock()
		b.c.resolver.wakemanual()
	}()
	if bytes.Equal(b.serversRaw, d) {
		return
	}
	b.serversRaw = d
	//offline app
	for _, exist := range b.servers {
		if _, ok := all[exist.addr]; !ok {
			//this app unregistered
			delete(b.servers, exist.addr)
		}
	}
	//online app or update app's dservers
	for addr, registerdata := range all {
		if len(registerdata.DServers) == 0 {
			delete(b.servers, addr)
			continue
		}
		exist, ok := b.servers[addr]
		if !ok {
			//this is a new register
			b.servers[addr] = &ServerForPick{
				addr: addr,
				client: &http.Client{
					Transport: &http.Transport{
						Proxy: http.ProxyFromEnvironment,
						DialContext: (&net.Dialer{
							Timeout:   b.c.c.ConnTimeout,
							KeepAlive: b.c.c.HeartProbe,
						}).DialContext,
						TLSClientConfig:        b.c.tlsc,
						ForceAttemptHTTP2:      true,
						MaxIdleConnsPerHost:    50,
						IdleConnTimeout:        b.c.c.IdleTimeout,
						MaxResponseHeaderBytes: int64(b.c.c.MaxHeader),
						ReadBufferSize:         int(b.c.c.SocketRBuf),
						WriteBufferSize:        int(b.c.c.SocketWBuf),
					},
					Timeout: b.c.c.GlobalTimeout,
				},
				dservers: registerdata.DServers,
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
				status: 1,
			}
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range exist.dservers {
				if _, ok := registerdata.DServers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range registerdata.DServers {
				if _, ok := exist.dservers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = 0
					break
				}
			}
			exist.dservers = registerdata.DServers
			exist.Pickinfo.Addition = registerdata.Addition
			exist.Pickinfo.DServerNum = int32(len(registerdata.DServers))
		}
	}
}
func (b *corelibBalancer) Pick(ctx context.Context) (*ServerForPick, error) {
	refresh := false
	for {
		b.lker.RLock()
		server := b.c.c.Picker(b.servers)
		b.lker.RUnlock()
		if server != nil {
			return server, nil
		}
		if refresh {
			return nil, ErrNoserver
		}
		if e := b.c.resolver.waitmanual(ctx); e != nil {
			if e == context.DeadlineExceeded {
				return nil, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return nil, cerror.ErrCanceled
			} else {
				return nil, cerror.ConvertStdError(e)
			}
		}
		refresh = true
	}
}