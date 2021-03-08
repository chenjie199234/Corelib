package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/metadata"
)

var ERRNOSERVER = errors.New("[web] no servers")

type PickHandler func(servers []*ServerForPick) *ServerForPick
type DiscoveryHandler func(appname string, client *WebClient)

type WebClient struct {
	timeout time.Duration

	lker  *sync.RWMutex
	hosts []*ServerForPick

	picker   PickHandler
	discover DiscoveryHandler
}
type ServerForPick struct {
	host             string
	client           *http.Client
	discoveryservers map[string]struct{} //this server registered on how many discoveryservers
	Pickinfo         *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nano second
	Activecalls    int32  //current active calls
	DServers       int32  //this server registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

var lker *sync.Mutex
var all map[string]*WebClient

func init() {
	rand.Seed(time.Now().UnixNano())
	lker = &sync.Mutex{}
	all = make(map[string]*WebClient)
}

func NewWebClient(appname string, globaltimeout time.Duration, picker PickHandler, discover DiscoveryHandler) (*WebClient, error) {
	if e := common.NameCheck(appname, true); e != nil {
		return nil, errors.New("[web.client]" + e.Error())
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
		timeout: globaltimeout,

		lker:  &sync.RWMutex{},
		hosts: make([]*ServerForPick, 0, 10),

		picker:   picker,
		discover: discover,
	}
	all[appname] = instance
	go instance.discover(appname, instance)
	return instance, nil
}

//firstkey:host
//secontkey:discovery server name
//value:addition data
func (this *WebClient) UpdateDiscovery(all map[string]map[string]struct{}, addition []byte) {
	this.lker.Lock()
	defer this.lker.Unlock()
	//check unregister
	pos := 0
	endpos := len(this.hosts) - 1
	for {
		if discoveryservers, ok := all[this.hosts[pos].host]; !ok || len(discoveryservers) == 0 {
			this.hosts[pos], this.hosts[endpos] = this.hosts[endpos], this.hosts[pos]
			endpos--
			if endpos < pos {
				break
			}
		} else {
			pos++
			if pos > endpos {
				break
			}
		}
	}
	this.hosts = this.hosts[:endpos+1]
	//check register
	for host, discoverservers := range all {
		if len(discoverservers) == 0 {
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
				discoveryservers: discoverservers,
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServers:       int32(len(discoverservers)),
					DServerOffline: 0,
					Addition:       addition,
				},
			})
			continue
		}
		//this is not a new register
		//unregister on which discovery server
		for dserver := range exist.discoveryservers {
			if _, ok := discoverservers[dserver]; !ok {
				delete(exist.discoveryservers, dserver)
				exist.Pickinfo.DServerOffline = time.Now().UnixNano()
			}
		}
		//register on which new discovery server
		for dserver := range discoverservers {
			if _, ok := exist.discoveryservers[dserver]; !ok {
				exist.discoveryservers[dserver] = struct{}{}
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
	if ok {
		req.Header.Set("Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	if md := metadata.GetAllMetadata(ctx); len(md) > 0 {
		d, _ := json.Marshal(md)
		req.Header.Set("Metadata", common.Byte2str(d))
	}
	atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
	defer atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
	resp, e := server.client.Do(req)
	if e != nil {
		atomic.StoreInt64(&server.Pickinfo.Lastfail, time.Now().UnixNano())
		return nil, e
	}
	return resp, nil
}
