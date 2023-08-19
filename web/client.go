package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
)

type ClientConfig struct {
	ConnectTimeout time.Duration //default 500ms
	GlobalTimeout  time.Duration //request's max handling time
	HeartProbe     time.Duration //tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout time.Duration
	MaxHeader   uint
}

func (c *ClientConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = time.Millisecond * 500
	}
	if c.HeartProbe == 0 {
		c.HeartProbe = time.Second * 15
	} else if c.HeartProbe > 0 && c.HeartProbe < time.Second {
		c.HeartProbe = time.Second
	}
	if c.MaxHeader == 0 {
		c.MaxHeader = 2048
	}
	if c.MaxHeader > 65536 {
		c.MaxHeader = 65536
	}
}

type WebClient struct {
	self   string
	server string
	c      *ClientConfig
	tlsc   *tls.Config
	client *http.Client

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be actived
func NewWebClient(c *ClientConfig, d discover.DI, selfproject, selfgroup, selfapp, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*WebClient, error) {
	//pre check
	if e := name.SingleCheck(selfproject, false); e != nil {
		return nil, e
	}
	if e := name.SingleCheck(selfgroup, false); e != nil {
		return nil, e
	}
	if e := name.SingleCheck(selfapp, false); e != nil {
		return nil, e
	}
	if e := name.SingleCheck(serverproject, false); e != nil {
		return nil, e
	}
	if e := name.SingleCheck(servergroup, false); e != nil {
		return nil, e
	}
	if e := name.SingleCheck(serverapp, false); e != nil {
		return nil, e
	}
	serverfullname := serverproject + "-" + servergroup + "." + serverapp
	selffullname := selfproject + "-" + selfgroup + "." + selfapp
	if c == nil {
		c = &ClientConfig{}
	}
	if d == nil {
		return nil, errors.New("[web.client] missing discover")
	}
	if !d.CheckApp(serverfullname) {
		return nil, errors.New("[web.client] discover's target app not match")
	}
	c.validate()
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   c.ConnectTimeout,
			KeepAlive: c.HeartProbe,
		}).DialContext,
		TLSClientConfig:        tlsc,
		TLSHandshakeTimeout:    c.ConnectTimeout,
		ForceAttemptHTTP2:      true,
		MaxIdleConnsPerHost:    128,
		IdleConnTimeout:        c.IdleTimeout,
		MaxResponseHeaderBytes: int64(c.MaxHeader),
	}
	if c.HeartProbe < 0 {
		transport.DisableKeepAlives = true
	}
	client := &WebClient{
		self:   selffullname,
		server: serverfullname,
		c:      c,
		tlsc:   tlsc,
		client: &http.Client{
			Transport: transport,
			Timeout:   c.GlobalTimeout,
		},

		discover: d,

		stop: graceful.New(),
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Web)
	return client, nil
}

func (c *WebClient) Close(force bool) {
	if force {
		c.resolver.Close()
		c.client.CloseIdleConnections()
	} else {
		c.stop.Close(c.resolver.Close, c.client.CloseIdleConnections)
	}
}

func forbiddenHeader(header http.Header) bool {
	if header == nil {
		return false
	}
	if header.Get("Core-Target") != "" {
		return true
	}
	if header.Get("Core-Deadline") != "" {
		return true
	}
	if header.Get("Core-Metadata") != "" {
		return true
	}
	if header.Get("Core-Tracedata") != "" {
		return true
	}
	return false
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Get(ctx context.Context, path, query string, header http.Header, metadata map[string]string) (resp *http.Response, e error) {
	return c.call(http.MethodGet, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Delete(ctx context.Context, path, query string, header http.Header, metadata map[string]string) (resp *http.Response, e error) {
	return c.call(http.MethodDelete, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Post(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPost, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPut, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPatch, ctx, path, query, header, metadata, nil)
}

func (c *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body io.Reader) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, cerror.MakeError(-1, 400, "forbidden header")
	}
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	if query != "" && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core-Target", c.server)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core-Metadata", common.Byte2str(d))
	}

	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid == "" {
		ctx = log.InitTrace(ctx, "", c.self, host.Hostip, "unknown", "unknown", 0)
		traceid, _, _, selfmethod, selfpath, selfdeep = log.GetTrace(ctx)
	}
	tracedata, _ := json.Marshal(map[string]string{
		"TraceID":      traceid,
		"SourceApp":    c.self,
		"SourceMethod": selfmethod,
		"SourcePath":   selfpath,
		"Deep":         strconv.Itoa(selfdeep),
	})
	header.Set("Core-Tracedata", common.Byte2str(tracedata))
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok {
		header.Set("Core-Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	header.Del("Origin")
	if !c.stop.AddOne() {
		return nil, cerror.ErrClientClosing
	}
	defer c.stop.DoneOne()
	for {
		start := time.Now()
		server, done, e := c.balancer.Pick(ctx)
		if e != nil {
			return nil, e
		}
		var req *http.Request
		if c.tlsc != nil {
			req, e = http.NewRequestWithContext(ctx, method, "https://"+server.addr+path+query, body)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, "http://"+server.addr+path+query, body)
		}
		if e != nil {
			done()
			e = cerror.ConvertStdError(e.(*url.Error).Unwrap())
			return nil, e
		}
		req.Header = header
		//start call
		resp, e := c.client.Do(req)
		done()
		end := time.Now()
		if e != nil {
			e = cerror.ConvertStdError(e.(*url.Error).Unwrap())
			log.Trace(ctx, log.CLIENT, c.server, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.server, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		if resp.StatusCode/100 != 2 {
			respbody, e := io.ReadAll(resp.Body)
			resp.Body.Close()
			if e != nil {
				e = cerror.ConvertStdError(e)
				log.Trace(ctx, log.CLIENT, c.server, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
				monitor.WebClientMonitor(c.server, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
				return nil, e
			}
			if len(respbody) == 0 {
				e = cerror.MakeError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				tmpe := cerror.ConvertErrorstr(common.Byte2str(respbody))
				tmpe.SetHttpcode(int32(resp.StatusCode))
				e = tmpe
			}
			if resp.StatusCode == int(cerror.ErrServerClosing.Httpcode) && cerror.Equal(e, cerror.ErrServerClosing) {
				log.Trace(ctx, log.CLIENT, c.server, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, cerror.ErrServerClosing)
				monitor.WebClientMonitor(c.server, method, path, cerror.ErrServerClosing, uint64(end.UnixNano()-start.UnixNano()))
				continue
			}
			log.Trace(ctx, log.CLIENT, c.server, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.server, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		log.Trace(ctx, log.CLIENT, c.server, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, nil)
		monitor.WebClientMonitor(c.server, method, path, nil, uint64(end.UnixNano()-start.UnixNano()))
		return resp, nil
	}
}
