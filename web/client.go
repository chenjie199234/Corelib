package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

var ERRNOSERVER = errors.New("[web] no servers")

type PickHandler func(servers map[string]*ServerForPick) *ServerForPick
type DiscoveryHandler func(group, name string, manually <-chan struct{}, client *WebClient)

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

	lker         *sync.RWMutex
	servers      map[string]*ServerForPick
	addrdata     []byte
	manually     chan struct{}
	manualNotice map[chan struct{}]struct{}
	mlker        *sync.Mutex
}
type ServerForPick struct {
	host     string
	client   *http.Client
	dservers map[string]struct{} //this server registered on how many discoveryservers
	Pickinfo *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nano second
	Activecalls    int32  //current active calls
	DServerNum     int32  //this server registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
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

		lker:         &sync.RWMutex{},
		servers:      make(map[string]*ServerForPick, 10),
		addrdata:     nil,
		manually:     make(chan struct{}, 1),
		manualNotice: make(map[chan struct{}]struct{}, 100),
		mlker:        &sync.Mutex{},
	}
	log.Info("[web.client] start finding server", group+"."+name)
	go c.Discover(group, name, client.manually, client)
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr
func (this *WebClient) UpdateDiscovery(all map[string]*RegisterData) {
	//check need update
	addrdata, _ := json.Marshal(all)
	this.lker.Lock()
	defer func() {
		this.mlker.Lock()
		for notice := range this.manualNotice {
			notice <- struct{}{}
			delete(this.manualNotice, notice)
		}
		this.mlker.Unlock()
		this.lker.Unlock()
	}()
	if bytes.Equal(this.addrdata, addrdata) {
		return
	}
	this.addrdata = addrdata
	//offline app
	for _, server := range this.servers {
		if _, ok := all[server.host]; !ok {
			//this app unregistered
			delete(this.servers, server.host)
		}
	}
	//online app or update app's dservers
	for host, registerdata := range all {
		if len(registerdata.DServers) == 0 {
			delete(this.servers, host)
		}
		if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
			continue
		} else if host == "http://" {
			continue
		} else if host == "https://" {
			continue
		}
		for len(host) > 0 && host[len(host)-1] == '/' {
			host = host[:len(host)-1]
		}
		if len(host) <= 6 {
			continue
		}
		exist, ok := this.servers[host]
		if !ok {
			//this is a new register
			this.servers[host] = &ServerForPick{
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
				dservers: registerdata.DServers,
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
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
	if len(pathwithquery) == 0 || pathwithquery[0] != '/' {
		pathwithquery = "/" + pathwithquery
	}
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
	manual := false
	for {
		var server *ServerForPick
		this.lker.RLock()
		server = this.c.Picker(this.servers)
		this.lker.RUnlock()
		if server == nil {
			if manual {
				return nil, ERRNOSERVER
			}
			this.mlker.Lock()
			manualNotice := make(chan struct{}, 1)
			this.manualNotice[manualNotice] = struct{}{}
			this.mlker.Unlock()
			//manual update server discover info
			select {
			case this.manually <- struct{}{}:
			default:
			}
			//wait manual update finish
			select {
			case <-manualNotice:
				manual = true
				continue
			case <-ctx.Done():
				this.mlker.Lock()
				delete(this.manualNotice, manualNotice)
				this.mlker.Unlock()
				return nil, ctx.Err()
			}
		}
		if ok && dl.UnixNano() < time.Now().UnixNano()+int64(5*time.Millisecond) {
			//ttl + server logic time
			return nil, context.DeadlineExceeded
		}
		var req *http.Request
		var e error
		if body != nil {
			req, e = http.NewRequestWithContext(ctx, method, server.host+pathwithquery, bytes.NewBuffer(body))
		} else {
			req, e = http.NewRequestWithContext(ctx, method, server.host+pathwithquery, nil)
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
			continue
		}
		return resp, nil
	}
}
