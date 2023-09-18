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
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
)

type ClientConfig struct {
	//the default timeout for every web call,<=0 means no timeout
	//if ctx's Deadline exist and GlobalTimeout > 0,the min(time.Now().Add(GlobalTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and GlobalTimeout > 0 ,the time.Now().Add(GlobalTimeout) will be used as the final deadline
	//if ctx's deadline not exist and GlobalTimeout <=0,means no deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time)
	//default 500ms
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 2048,max 65536,unit byte
	MaxResponseHeader uint `json:"max_response_header"`
}

func (c *ClientConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
	}
	if c.MaxResponseHeader == 0 {
		c.MaxResponseHeader = 2048
	} else if c.MaxResponseHeader > 65536 {
		c.MaxResponseHeader = 65536
	}
}

type WebClient struct {
	self   string
	server string
	c      *ClientConfig
	tlsc   *tls.Config
	dialer *net.Dialer
	client *http.Client

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be actived
func NewWebClient(c *ClientConfig, d discover.DI, selfproject, selfgroup, selfapp, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*WebClient, error) {
	if tlsc != nil {
		tlsc = tlsc.Clone()
	}
	//pre check
	serverfullname, e := name.MakeFullName(serverproject, servergroup, serverapp)
	if e != nil {
		return nil, e
	}
	selffullname, e := name.MakeFullName(selfproject, selfgroup, selfapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ClientConfig{}
	}
	c.validate()
	if d == nil {
		return nil, errors.New("[web.client] missing discover")
	}
	if !d.CheckTarget(serverfullname) {
		return nil, errors.New("[web.client] discover's target app not match")
	}

	client := &WebClient{
		self:   selffullname,
		server: serverfullname,
		c:      c,
		tlsc:   tlsc,
		dialer: &net.Dialer{},

		discover: d,

		stop: graceful.New(),
	}
	client.client = &http.Client{
		Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			DialContext:            client.dial,
			DialTLSContext:         client.dialtls,
			TLSClientConfig:        tlsc,
			ForceAttemptHTTP2:      true,
			MaxIdleConnsPerHost:    256,
			IdleConnTimeout:        c.IdleTimeout.StdDuration(),
			MaxResponseHeaderBytes: int64(c.MaxResponseHeader),
		},
		Timeout: c.GlobalTimeout.StdDuration(),
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Web)
	return client, nil
}

// this is for http.Transport
func (c *WebClient) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if c.c.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.c.ConnectTimeout.StdDuration())
		defer cancel()
	}
	conn, e := c.dialer.DialContext(ctx, network, addr)
	if e != nil {
		log.Error(ctx, "[web.client] dial failed", map[string]interface{}{"sname": c.server, "sip": addr, "error": e})
	} else {
		log.Info(ctx, "[web.client] online", map[string]interface{}{"sname": c.server, "sip": addr})
	}
	return conn, e
}

// this is for http.Transport
func (c *WebClient) dialtls(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	if c.c.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.c.ConnectTimeout.StdDuration())
		defer cancel()
	}
	conn, e := c.dialer.DialContext(ctx, network, addr)
	if e != nil {
		log.Error(ctx, "[web.client] dial failed", map[string]interface{}{"sname": c.server, "sip": addr, "error": e})
		return nil, e
	}
	tmptlsc := c.tlsc.Clone()
	if tmptlsc.ServerName == "" {
		tmptlsc.ServerName = host
	}
	tc := tls.Client(conn, tmptlsc)
	if e = tc.HandshakeContext(ctx); e != nil {
		log.Error(ctx, "[web.client] tls handshake failed", map[string]interface{}{"sname": c.server, "sip": addr, "error": e})
		return nil, e
	} else {
		log.Info(ctx, "[web.client] online", map[string]interface{}{"sname": c.server, "sip": addr})
	}
	return tc, nil
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
		return c.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewReader(body))
	}
	return c.call(http.MethodPost, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewReader(body))
	}
	return c.call(http.MethodPut, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Metadata" "Core-Tracedata" are forbidden in header
func (c *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewReader(body))
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
	var dl time.Time
	var ok bool
	if dl, ok = ctx.Deadline(); ok {
		if c.c.GlobalTimeout > 0 {
			clientdl := time.Now().Add(c.c.GlobalTimeout.StdDuration())
			if dl.After(clientdl) {
				dl = clientdl
			}
		}
	} else if c.c.GlobalTimeout > 0 {
		dl = time.Now().Add(c.c.GlobalTimeout.StdDuration())
	}
	if !dl.IsZero() {
		header.Set("Core-Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return nil, cerror.ErrClientClosing
		}
		return nil, cerror.ErrBusy
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
