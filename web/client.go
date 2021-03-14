package web

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
)

var ERRNOSERVER = errors.New("[web] no servers")

type PickHandler func(servers []*ServerForPick) *ServerForPick
type DiscoveryHandler func(group, name string, client *WebClient)

type WebClient struct {
	selfappname string
	appname     string
	timeout     time.Duration

	lker  *sync.RWMutex
	hosts []*ServerForPick

	picker   PickHandler
	discover DiscoveryHandler
}
type ServerForPick struct {
	host             string
	client           *http.Client
	discoveryservers []string //this server registered on how many discoveryservers
	closing          bool
	Pickinfo         *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nano second
	Activecalls    int32  //current active calls
	DServers       int32  //this server registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

func (s *ServerForPick) Pickable() bool {
	return !s.closing
}

var lker *sync.Mutex
var all map[string]*WebClient

func init() {
	rand.Seed(time.Now().UnixNano())
	lker = &sync.Mutex{}
	all = make(map[string]*WebClient)
}

func NewWebClient(globaltimeout time.Duration, selfgroup, selfname, group, name string, picker PickHandler, discover DiscoveryHandler) (*WebClient, error) {
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	appname := group + "." + name
	if e := common.NameCheck(appname, true, true, false, true); e != nil {
		return nil, e
	}
	selfappname := selfgroup + "." + selfname
	if e := common.NameCheck(selfappname, true, true, false, true); e != nil {
		return nil, e
	}
	lker.Lock()
	defer lker.Unlock()
	if c, ok := all[appname]; ok {
		return c, nil
	}
	if picker == nil {
		picker = defaultPicker
	}
	if discover == nil {
		discover = defaultDiscover
	}
	instance := &WebClient{
		selfappname: selfappname,
		appname:     appname,
		timeout:     globaltimeout,

		lker:  &sync.RWMutex{},
		hosts: make([]*ServerForPick, 0, 10),

		picker:   picker,
		discover: discover,
	}
	all[appname] = instance
	go instance.discover(group, name, instance)
	return instance, nil
}

//all:
//key:addr
//value:discovery servers
//addition
//addition info
func (this *WebClient) UpdateDiscovery(all map[string][]string, addition []byte) {
	this.lker.Lock()
	defer this.lker.Unlock()
	//check unregister
	pos := 0
	endpos := len(this.hosts) - 1
	if this.hosts != nil {
		for pos <= endpos {
			existhost := this.hosts[pos]
			if discoveryservers, ok := all[existhost.host]; !ok || len(discoveryservers) == 0 {
				if pos != endpos {
					this.hosts[pos], this.hosts[endpos] = this.hosts[endpos], this.hosts[pos]
				}
				endpos--
			} else {
				pos++
			}
		}
		if pos != len(this.hosts) {
			this.hosts = this.hosts[:pos]
		}
	}
	if this.hosts == nil {
		this.hosts = make([]*ServerForPick, 0, 5)
	}
	//check register
	for host, discoveryservers := range all {
		if len(discoveryservers) == 0 {
			continue
		}
		var exist *ServerForPick
		for _, existhost := range this.hosts {
			if existhost.host == host {
				exist = existhost
				break
			}
		}
		if exist == nil {
			//this is a new register
			this.hosts = append(this.hosts, &ServerForPick{
				host:             host,
				client:           &http.Client{},
				discoveryservers: discoveryservers,
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServers:       int32(len(discoveryservers)),
					DServerOffline: 0,
					Addition:       addition,
				},
			})
			continue
		}
		//this is not a new register
		//unregister on which discovery server
		if exist.discoveryservers != nil {
			pos = 0
			endpos = len(exist.discoveryservers) - 1
			for pos <= endpos {
				existdserver := exist.discoveryservers[pos]
				find := false
				for _, newdserver := range discoveryservers {
					if newdserver == existdserver {
						find = true
						break
					}
				}
				if find {
					pos++
				} else {
					if pos != endpos {
						exist.discoveryservers[pos], exist.discoveryservers[endpos] = exist.discoveryservers[endpos], exist.discoveryservers[pos]
					}
					endpos--
				}
			}
			if pos != len(exist.discoveryservers) {
				exist.Pickinfo.DServerOffline = time.Now().UnixNano()
				exist.discoveryservers = exist.discoveryservers[:pos]
			}
		}
		if exist.discoveryservers == nil {
			exist.discoveryservers = make([]string, 0, 5)
		}
		//register on which new discovery server
		for _, newdserver := range discoveryservers {
			find := false
			for _, existdserver := range exist.discoveryservers {
				if existdserver == newdserver {
					find = true
					break
				}
			}
			if !find {
				exist.discoveryservers = append(exist.discoveryservers, newdserver)
				exist.Pickinfo.DServerOffline = 0
			}
		}
		//
		exist.Pickinfo.Addition = addition
		exist.Pickinfo.DServers = int32(len(exist.discoveryservers))
	}
}

func (this *WebClient) Get(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header) (*http.Response, error) {
	return this.call(http.MethodGet, ctx, functimeout, pathwithquery, header, nil)
}
func (this *WebClient) Delete(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header) (*http.Response, error) {
	return this.call(http.MethodDelete, ctx, functimeout, pathwithquery, header, nil)
}
func (this *WebClient) Post(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, body []byte) (*http.Response, error) {
	return this.call(http.MethodPost, ctx, functimeout, pathwithquery, header, body)
}
func (this *WebClient) Put(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, body []byte) (*http.Response, error) {
	return this.call(http.MethodPut, ctx, functimeout, pathwithquery, header, body)
}
func (this *WebClient) Patch(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, body []byte) (*http.Response, error) {
	return this.call(http.MethodPatch, ctx, functimeout, pathwithquery, header, body)
}
func (this *WebClient) call(method string, ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, body []byte) (*http.Response, error) {
	if header == nil {
		header = make(http.Header)
	}
	header.Set("TargetServer", this.appname)
	header.Set("SourceServer", this.selfappname)
	var min time.Duration
	if this.timeout != 0 {
		min = this.timeout
	}
	if functimeout != 0 {
		if min == 0 {
			min = functimeout
		} else if functimeout < min {
			min = functimeout
		}
	}
	if min != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, min)
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok {
		header.Set("Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	header.Del("Origin")
	for {
		if ok && dl.UnixNano() < time.Now().UnixNano()+int64(5*time.Millisecond) {
			//ttl + server logic time
			return nil, context.DeadlineExceeded
		}
		var server *ServerForPick
		this.lker.RLock()
		server = this.picker(this.hosts)
		this.lker.RUnlock()
		if server == nil {
			return nil, ERRNOSERVER
		}
		add := false
		del := false
		if server.host[len(server.host)-1] == '/' && pathwithquery[0] == '/' {
			del = true
		} else if server.host[len(server.host)-1] != '/' && pathwithquery[0] != '/' {
			add = true
		}
		url := ""
		if add {
			url = server.host + "/" + pathwithquery
		} else if del {
			url = server.host + pathwithquery[1:]
		} else {
			url = server.host + pathwithquery
		}
		var req *http.Request
		var e error
		if body != nil {
			req, e = http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
		} else {
			req, e = http.NewRequestWithContext(ctx, method, url, nil)
		}
		if e != nil {
			return nil, e
		}
		req.Header = header
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		resp, e := server.client.Do(req)
		atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
		if e != nil {
			atomic.StoreInt64(&server.Pickinfo.Lastfail, time.Now().UnixNano())
			return nil, e
		}
		if resp.StatusCode == 888 {
			atomic.StoreInt64(&server.Pickinfo.Lastfail, time.Now().UnixNano())
			server.closing = true
			continue
		}
		return resp, nil
	}
}
