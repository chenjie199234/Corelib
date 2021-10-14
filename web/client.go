package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
)

//param's key is server's addr "scheme://host:port"
type PickHandler func(servers map[string]*ServerForPick) *ServerForPick

//return data's key is server's addr "scheme://host:port"
type DiscoveryHandler func(group, name string, manually <-chan *struct{}, client *WebClient)

type ClientConfig struct {
	ConnTimeout   time.Duration
	GlobalTimeout time.Duration //request's max handling time
	HeartProbe    time.Duration //tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout      time.Duration
	MaxHeader        uint
	SocketRBuf       uint
	SocketWBuf       uint
	UseTLS           bool     //http or https
	SkipVerifyTLS    bool     //don't verify the server's cert
	CAs              []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	DiscoverFunction DiscoveryHandler //this function will be called in goroutine in NewWebClient
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
}

type WebClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	//certpool    *x509.CertPool
	tlsc *tls.Config

	lker       *sync.RWMutex
	serversRaw []byte
	servers    map[string]*ServerForPick

	manually     chan *struct{}
	manualNotice map[chan *struct{}]*struct{}
	mlker        *sync.Mutex
}
type ServerForPick struct {
	host     string
	client   *http.Client
	dservers map[string]struct{} //this server registered on how many discoveryservers
	status   bool                //true - working,false - closed

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
	return s.status
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
		log.Warning(nil, "[web.client] missing picker in config,default picker will be used")
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
		tlsc: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},

		lker:         &sync.RWMutex{},
		servers:      make(map[string]*ServerForPick, 10),
		manually:     make(chan *struct{}, 1),
		manualNotice: make(map[chan *struct{}]*struct{}, 100),
		mlker:        &sync.Mutex{},
	}
	//init discover
	go c.DiscoverFunction(group, name, client.manually, client)
	client.waitmanual(context.Background())
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

func (c *WebClient) manual() {
	select {
	case c.manually <- nil:
	default:
	}
}
func (c *WebClient) waitmanual(ctx context.Context) error {
	notice := make(chan *struct{}, 1)
	c.mlker.Lock()
	c.manualNotice[notice] = nil
	c.manual()
	c.mlker.Unlock()
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		c.mlker.Lock()
		delete(c.manualNotice, notice)
		c.mlker.Unlock()
		return ctx.Err()
	}
}
func (c *WebClient) wakemanual() {
	c.mlker.Lock()
	for notice := range c.manualNotice {
		notice <- nil
		delete(c.manualNotice, notice)
	}
	c.mlker.Unlock()
}

//all: key server's addr "host:port"
func (this *WebClient) UpdateDiscovery(all map[string]*RegisterData) {
	d, _ := json.Marshal(all)
	this.lker.Lock()
	if bytes.Equal(this.serversRaw, d) {
		this.lker.Unlock()
		this.wakemanual()
		return
	}
	this.serversRaw = d
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
		exist, ok := this.servers[host]
		if !ok {
			//this is a new register
			this.servers[host] = &ServerForPick{
				host: host,
				client: &http.Client{
					Transport: &http.Transport{
						Proxy: http.ProxyFromEnvironment,
						DialContext: (&net.Dialer{
							Timeout:   this.c.ConnTimeout,
							KeepAlive: this.c.HeartProbe,
						}).DialContext,
						TLSClientConfig:        this.tlsc,
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
				status: true,
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
	this.lker.Unlock()
	this.wakemanual()
}

func forbiddenHeader(header http.Header) bool {
	if _, ok := header["SourceApp"]; ok {
		return true
	}
	if _, ok := header["SourcePath"]; ok {
		return true
	}
	if _, ok := header["SourceMethod"]; ok {
		return true
	}
	if _, ok := header["Deadline"]; ok {
		return true
	}
	if _, ok := header["Metadata"]; ok {
		return true
	}
	if _, ok := header["Traceid"]; ok {
		return true
	}
	return false
}

//"SourceApp" "SourcePath" "SourceMethod" "Deadline" "Metadata" "Traceid" are forbidden in header
func (this *WebClient) Get(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodGet, ctx, functimeout, path, query, header, metadata, nil)
}

//"SourceApp" "SourcePath" "SourceMethod" "Deadline" "Metadata" "Traceid" are forbidden in header
func (this *WebClient) Delete(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodDelete, ctx, functimeout, path, query, header, metadata, nil)
}

//"SourceApp" "SourcePath" "SourceMethod" "Deadline" "Metadata" "Traceid" are forbidden in header
func (this *WebClient) Post(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPost, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPost, ctx, functimeout, path, query, header, metadata, nil)
}

//"SourceApp" "SourcePath" "SourceMethod" "Deadline" "Metadata" "Traceid" are forbidden in header
func (this *WebClient) Put(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPut, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPut, ctx, functimeout, path, query, header, metadata, nil)
}

//"SourceApp" "SourcePath" "SourceMethod" "Deadline" "Metadata" "Traceid" are forbidden in header
func (this *WebClient) Patch(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPatch, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPatch, ctx, functimeout, path, query, header, metadata, nil)
}
func (this *WebClient) call(method string, ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body *bytes.Buffer) (*http.Response, error) {
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	if len(query) != 0 && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("SourceApp", this.selfappname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath := trace.GetTrace(ctx)
	if traceid != "" {
		header.Set("Traceid", traceid)
		header.Set("SourcePath", selfpath)
		header.Set("SourceMethod", selfmethod)
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
	for {
		server, e := this.pick(ctx)
		if e != nil {
			return nil, e
		}
		start := time.Now()
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		if ok && dl.UnixNano() < start.UnixNano()+int64(5*time.Millisecond) {
			//ttl + server logic time
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			return nil, cerror.ErrDeadlineExceeded
		}
		var scheme string
		if this.c.UseTLS {
			scheme = "https"
		} else {
			scheme = "http"
		}
		var req *http.Request
		if body == nil {
			req, e = http.NewRequestWithContext(ctx, method, scheme+"://"+server.host+path+query, nil)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, scheme+"://"+server.host+path+query, body)
		}
		if e != nil {
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			return nil, cerror.ConvertStdError(e)
		}
		req.Header = header
		//start call
		var resp *http.Response
		resp, e = server.client.Do(req)
		atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
		end := time.Now()
		if e != nil {
			server.Pickinfo.Lastfail = time.Now().UnixNano()
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, this.appname, server.host, method, path, &start, &end, e)
			return nil, e
		}
		if resp.StatusCode == 888 {
			server.Pickinfo.Lastfail = time.Now().UnixNano()
			this.manual()
			server.status = false
			resp.Body.Close()
			trace.Trace(ctx, trace.CLIENT, this.appname, server.host, method, path, &start, &end, ERRCLOSING)
			continue
		}
		if resp.StatusCode != 200 {
			var respbody []byte
			respbody, e = io.ReadAll(resp.Body)
			resp.Body.Close()
			if e != nil {
				e = cerror.ConvertStdError(e)
				trace.Trace(ctx, trace.CLIENT, this.appname, server.host, method, path, &start, &end, e)
				return nil, e
			}
			if len(respbody) == 0 {
				if text := http.StatusText(resp.StatusCode); text != "" {
					e = cerror.MakeError(-1, text)
				} else {
					e = cerror.MakeError(-1, "http code:"+strconv.FormatInt(int64(resp.StatusCode), 10)+",response body:empty")
				}
			} else {
				e = cerror.ConvertErrorstr(common.Byte2str(respbody))
			}
			trace.Trace(ctx, trace.CLIENT, this.appname, server.host, method, path, &start, &end, e)
			return nil, e
		}
		trace.Trace(ctx, trace.CLIENT, this.appname, server.host, method, path, &start, &end, nil)
		return resp, nil
	}
}
func (this *WebClient) pick(ctx context.Context) (*ServerForPick, error) {
	refresh := false
	for {
		this.lker.RLock()
		server := this.c.Picker(this.servers)
		this.lker.RUnlock()
		if server != nil {
			return server, nil
		}
		if refresh {
			return nil, ERRNOSERVER
		}
		if e := this.waitmanual(ctx); e != nil {
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
