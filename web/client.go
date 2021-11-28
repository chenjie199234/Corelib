package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
)

type PickHandler func(servers []*ServerForPick) *ServerForPick

type DiscoveryHandler func(group, name string, manually <-chan *struct{}, client *WebClient)

type ClientConfig struct {
	ConnTimeout   time.Duration
	GlobalTimeout time.Duration //request's max handling time(including connection establish time)
	HeartProbe    time.Duration //tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout   time.Duration
	MaxHeader     uint
	SocketRBuf    uint
	SocketWBuf    uint
	UseTLS        bool     //http or https
	SkipVerifyTLS bool     //don't verify the server's cert
	CAs           []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker        PickHandler
	Discover      DiscoveryHandler //this function will be called in goroutine in NewWebClient
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
	tlsc        *tls.Config

	resolver *corelibResolver
	balancer *corelibBalancer
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
	}
	client.balancer = newCorelibBalancer(client)
	//init discover
	client.resolver = newCorelibResolver(group, name, client)
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr "host:port"
func (this *WebClient) UpdateDiscovery(all map[string]*RegisterData) {
	this.balancer.UpdateDiscovery(all)
}

func forbiddenHeader(header http.Header) bool {
	if _, ok := header[textproto.CanonicalMIMEHeaderKey("core_target")]; ok {
		return true
	}
	if _, ok := header[textproto.CanonicalMIMEHeaderKey("core_deadline")]; ok {
		return true
	}
	if _, ok := header[textproto.CanonicalMIMEHeaderKey("core_metadata")]; ok {
		return true
	}
	if _, ok := header[textproto.CanonicalMIMEHeaderKey("core_tracedata")]; ok {
		return true
	}
	return false
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Get(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodGet, ctx, functimeout, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Delete(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodDelete, ctx, functimeout, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Post(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPost, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPost, ctx, functimeout, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Put(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPut, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPut, ctx, functimeout, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Patch(ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPatch, ctx, functimeout, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPatch, ctx, functimeout, path, query, header, metadata, nil)
}
func (this *WebClient) call(method string, ctx context.Context, functimeout time.Duration, path, query string, header http.Header, metadata map[string]string, body *bytes.Buffer) ([]byte, error) {
	start := time.Now()
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	if len(query) != 0 && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core_target", this.appname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core_metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath := trace.GetTrace(ctx)
	if traceid != "" {
		header.Set("Core_tracedata", traceid)
		header.Add("Core_tracedata", this.selfappname)
		header.Add("Core_tracedata", selfmethod)
		header.Add("Core_tracedata", selfpath)
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
		if dl.UnixNano() < start.UnixNano()+int64(time.Millisecond*5) {
			return nil, cerror.ErrDeadlineExceeded
		}
		header.Set("Core_deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	header.Del("Origin")
	for {
		server, e := this.balancer.Pick(ctx)
		if e != nil {
			return nil, e
		}
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		if ok && dl.UnixNano() < start.UnixNano()+int64(5*time.Millisecond) {
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
			req, e = http.NewRequestWithContext(ctx, method, scheme+"://"+server.addr+path+query, nil)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, scheme+"://"+server.addr+path+query, body)
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
			server.Pickinfo.LastFailTime = time.Now().UnixNano()
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, this.appname, server.addr, method, path, &start, &end, e)
			return nil, e
		}
		if resp.StatusCode == 888 {
			server.Pickinfo.LastFailTime = time.Now().UnixNano()
			server.setclient(nil)
			this.balancer.RebuildPicker()
			this.resolver.manual(nil)
			resp.Body.Close()
			trace.Trace(ctx, trace.CLIENT, this.appname, server.addr, method, path, &start, &end, errClosing)
			continue
		}
		respbody, e := io.ReadAll(resp.Body)
		resp.Body.Close()
		if e != nil {
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, this.appname, server.addr, method, path, &start, &end, e)
			return nil, e
		}
		if resp.StatusCode != 200 {
			if len(respbody) == 0 {
				e = cerror.MakeError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				tempe := cerror.ConvertErrorstr(common.Byte2str(respbody))
				tempe.Httpcode = int32(resp.StatusCode)
				e = tempe
			}
			trace.Trace(ctx, trace.CLIENT, this.appname, server.addr, method, path, &start, &end, e)
			return nil, e
		}
		trace.Trace(ctx, trace.CLIENT, this.appname, server.addr, method, path, &start, &end, nil)
		return respbody, nil
	}
}
