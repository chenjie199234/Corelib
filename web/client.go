package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

var ERRNOSERVER = errors.New("[web] no servers")

type PickHandler func(servers []*ServerForPick) *ServerForPick
type DiscoveryHandler func(group, name string, client *WebClient)

type ClientConfig struct {
	//request's max handling time
	GlobalTimeout time.Duration
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout time.Duration
	//system's tcp keep alive probe interval,negative disable keep alive,0 will be set to default 15s
	HeartProbe    time.Duration
	MaxHeader     uint
	SocketRBuf    uint
	SocketWBuf    uint
	SkipVerifyTLS bool     //don't verify the server's cert
	CAs           []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker        PickHandler
	Discover      DiscoveryHandler
}

func (c *ClientConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartProbe == 0 {
		c.HeartProbe = time.Second * 15
	}
	if c.MaxHeader == 0 {
		c.MaxHeader = 1024
	}
	if c.MaxHeader > 65535 {
		c.MaxHeader = 65535
	}
	if c.SocketRBuf == 0 {
		c.SocketRBuf = 1024
	}
	if c.SocketRBuf > 65535 {
		c.SocketRBuf = 65535
	}
	if c.SocketWBuf == 0 {
		c.SocketWBuf = 1024
	}
	if c.SocketWBuf > 65535 {
		c.SocketWBuf = 65535
	}
}

type WebClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	certpool    *x509.CertPool

	lker  *sync.RWMutex
	hosts []*ServerForPick
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

func NewWebClient(c *ClientConfig, selfgroup, selfname, group, name string) (*WebClient, error) {
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
	if c == nil {
		return nil, errors.New("[web.client] missing config")
	}
	if c.Discover == nil {
		return nil, errors.New("[web.client] missing discover in config")
	}
	if c.Picker == nil {
		log.Warning("[web.client] missing picker in config,default picker will be used")
		c.Picker = defaultPicker
	}
	c.validate()
	var certpool *x509.CertPool
	if len(c.CAs) != 0 {
		certpool = x509.NewCertPool()
		for _, cert := range c.CAs {
			certPEM, e := os.ReadFile(cert)
			if e != nil {
				return nil, errors.New("[web.client] read cert file:" + cert + " error:" + e.Error())
			}
			if !certpool.AppendCertsFromPEM(certPEM) {
				return nil, errors.New("[web.client] load cert file:" + cert + " error:" + e.Error())
			}
		}
	}
	client := &WebClient{
		selfappname: selfappname,
		appname:     appname,
		c:           c,
		certpool:    certpool,

		lker:  &sync.RWMutex{},
		hosts: make([]*ServerForPick, 0, 10),
	}
	log.Info("[web.client] start finding server", group+"."+name)
	go c.Discover(group, name, client)
	return client, nil
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
	head := 0
	tail := len(this.hosts) - 1
	for head <= tail {
		existhost := this.hosts[head]
		if discoveryservers, ok := all[existhost.host]; !ok || len(discoveryservers) == 0 {
			if head != tail {
				this.hosts[head], this.hosts[tail] = this.hosts[tail], this.hosts[head]
			}
			tail--
		} else {
			head++
		}
	}
	if this.hosts != nil && head != len(this.hosts) {
		this.hosts = this.hosts[:head]
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
		//this is a new register
		if exist == nil {
			this.hosts = append(this.hosts, &ServerForPick{
				host: host,
				client: &http.Client{
					Transport: &http.Transport{
						Proxy: http.ProxyFromEnvironment,
						DialContext: (&net.Dialer{
							KeepAlive: this.c.HeartProbe,
							Control: func(network, address string, c syscall.RawConn) error {
								c.Control(func(fd uintptr) {
									syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, int(this.c.SocketRBuf))
									syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, int(this.c.SocketWBuf))
								})
								return nil
							},
						}).DialContext,
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: this.c.SkipVerifyTLS,
							RootCAs:            this.certpool,
						},
						ForceAttemptHTTP2:      true,
						MaxIdleConnsPerHost:    50,
						IdleConnTimeout:        this.c.IdleTimeout,
						MaxResponseHeaderBytes: int64(this.c.MaxHeader),
						ReadBufferSize:         int(this.c.SocketRBuf),
						WriteBufferSize:        int(this.c.SocketWBuf),
					},
					Timeout: this.c.GlobalTimeout,
				},
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
		head = 0
		tail = len(exist.discoveryservers) - 1
		for head <= tail {
			existdserver := exist.discoveryservers[head]
			find := false
			for _, newdserver := range discoveryservers {
				if newdserver == existdserver {
					find = true
					break
				}
			}
			if find {
				head++
			} else {
				if head != tail {
					exist.discoveryservers[head], exist.discoveryservers[tail] = exist.discoveryservers[tail], exist.discoveryservers[head]
				}
				tail--
			}
		}
		if exist.discoveryservers != nil && head != len(exist.discoveryservers) {
			exist.Pickinfo.DServerOffline = time.Now().UnixNano()
			exist.discoveryservers = exist.discoveryservers[:head]
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
	if this.c.GlobalTimeout != 0 {
		min = this.c.GlobalTimeout
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
		server = this.c.Picker(this.hosts)
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
