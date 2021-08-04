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
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	cerror "github.com/chenjie199234/Corelib/util/error"
)

var ERRNOSERVER = errors.New("[web] no servers")
var ERRHEADER = errors.New("[web] forbidden header")

//param's key is server's addr "scheme://host:port"
type PickHandler func(servers map[string]*ServerForPick) *ServerForPick

//return data's key is server's addr "scheme://host:port"
type DiscoveryHandler func(group, name string) map[string]*RegisterData

type ClientConfig struct {
	//request's max handling time
	GlobalTimeout time.Duration
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout time.Duration
	//system's tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	HeartProbe       time.Duration
	MaxHeader        uint
	SocketRBuf       uint
	SocketWBuf       uint
	SkipVerifyTLS    bool     //don't verify the server's cert
	CAs              []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	DiscoverFunction DiscoveryHandler
	DiscoverInterval time.Duration //min 1 second
}

func (c *ClientConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartProbe == 0 {
		c.HeartProbe = time.Second * 15
	} else if c.HeartProbe > 0 && c.HeartProbe < time.Second {
		c.HeartProbe = time.Second
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
	if c.DiscoverInterval < time.Second {
		c.DiscoverInterval = time.Second
	}
}

type WebClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	certpool    *x509.CertPool

	lker    *sync.RWMutex
	servers map[string]*ServerForPick

	manually     chan struct{}
	manualNotice map[chan struct{}]struct{}
	mlker        *sync.Mutex
}
type ServerForPick struct {
	host     string
	client   *http.Client
	dservers map[string]struct{} //this server registered on how many discoveryservers
	status   int                 //1-working,0-closing

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
	return s.status == 1
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
	if c.DiscoverFunction == nil {
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
		manually:     make(chan struct{}, 1),
		manualNotice: make(map[chan struct{}]struct{}, 100),
		mlker:        &sync.Mutex{},
	}
	//init discover
	log.Info("[web.client] start discovering server", group+"."+name)
	client.manually <- struct{}{}
	manualNotice := make(chan struct{}, 1)
	client.manualNotice[manualNotice] = struct{}{}
	go defaultDiscover(group, name, client)
	<-manualNotice
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr "scheme://host:port"
func (this *WebClient) UpdateDiscovery(all map[string]*RegisterData) {
	//check need update
	this.lker.Lock()
	defer this.lker.Unlock()
	//offline app
	for _, exist := range this.servers {
		if _, ok := all[exist.host]; !ok {
			//this app unregistered
			delete(this.servers, exist.host)
		}
	}
	//online app or update app's dservers
	for host, registerdata := range all {
		if len(registerdata.DServers) == 0 {
			delete(this.servers, host)
			continue
		}
		if u, e := url.Parse(host); e != nil {
			continue
		} else if u.Host == "" || u.Scheme == "" || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		} else {
			host = u.Scheme + "://" + u.Host
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

func forbiddenHeader(header http.Header) bool {
	if _, ok := header["TargetServer"]; ok {
		return true
	}
	if _, ok := header["SourceServer"]; ok {
		return true
	}
	if _, ok := header["Deadline"]; ok {
		return true
	}
	if _, ok := header["Metadata"]; ok {
		return true
	}
	return false
}

//"TargetServer" "SourceServer" "Deadline" and "Metadata" are forbidden in header
func (this *WebClient) Get(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, ERRHEADER
	}
	return this.call(http.MethodGet, ctx, functimeout, pathwithquery, header, metadata, nil)
}

//"TargetServer" "SourceServer" "Deadline" and "Metadata" are forbidden in header
func (this *WebClient) Delete(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, ERRHEADER
	}
	return this.call(http.MethodDelete, ctx, functimeout, pathwithquery, header, metadata, nil)
}

//"TargetServer" "SourceServer" "Deadline" and "Metadata" are forbidden in header
func (this *WebClient) Post(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, ERRHEADER
	}
	return this.call(http.MethodPost, ctx, functimeout, pathwithquery, header, metadata, body)
}

//"TargetServer" "SourceServer" "Deadline" and "Metadata" are forbidden in header
func (this *WebClient) Put(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, ERRHEADER
	}
	return this.call(http.MethodPut, ctx, functimeout, pathwithquery, header, metadata, body)
}

//"TargetServer" "SourceServer" "Deadline" and "Metadata" are forbidden in header
func (this *WebClient) Patch(ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, ERRHEADER
	}
	return this.call(http.MethodPatch, ctx, functimeout, pathwithquery, header, metadata, body)
}
func (this *WebClient) call(method string, ctx context.Context, functimeout time.Duration, pathwithquery string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if len(pathwithquery) == 0 || pathwithquery[0] != '/' {
		pathwithquery = "/" + pathwithquery
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("TargetServer", this.appname)
	header.Set("SourceServer", this.selfappname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Metadata", common.Byte2str(d))
	}
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
			if len(this.manualNotice) == 1 {
				this.manually <- struct{}{}
			}
			this.mlker.Unlock()
			//wait manual update finish
			select {
			case <-manualNotice:
				manual = true
				continue
			case <-ctx.Done():
				this.mlker.Lock()
				delete(this.manualNotice, manualNotice)
				this.mlker.Unlock()
				return nil, cerror.StdErrorToError(ctx.Err())
			}
		}
		if !server.Pickable() {
			continue
		}
		if ok && dl.UnixNano() < time.Now().UnixNano()+int64(5*time.Millisecond) {
			//ttl + server logic time
			return nil, cerror.StdErrorToError(context.DeadlineExceeded)
		}
		var req *http.Request
		var e error
		if body != nil {
			req, e = http.NewRequestWithContext(ctx, method, server.host+pathwithquery, bytes.NewBuffer(body))
		} else {
			req, e = http.NewRequestWithContext(ctx, method, server.host+pathwithquery, nil)
		}
		if e != nil {
			return nil, cerror.StdErrorToError(e)
		}
		req.Header = header
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		resp, e := server.client.Do(req)
		atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
		if e != nil {
			server.Pickinfo.Lastfail = time.Now().UnixNano()
			return nil, cerror.StdErrorToError(e)
		}
		if resp.StatusCode == 888 {
			server.Pickinfo.Lastfail = time.Now().UnixNano()
			server.status = 0
			continue
		}
		return resp, nil
	}
}
