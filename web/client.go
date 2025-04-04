package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type ClientConfig struct {
	//the default timeout for every web call,<=0 means no timeout
	//if ctx's Deadline exist and GlobalTimeout > 0,the min(time.Now().Add(GlobalTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and GlobalTimeout > 0 ,the time.Now().Add(GlobalTimeout) will be used as the final deadline
	//if ctx's deadline not exist and GlobalTimeout <=0,means no deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 2048,max 65536,unit byte
	MaxResponseHeader uint `json:"max_response_header"`
}

func (c *ClientConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(time.Second * 3)
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
	serverfullname string
	c              *ClientConfig
	tlsc           *tls.Config
	dialer         *net.Dialer
	client         *http.Client

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be actived
func NewWebClient(c *ClientConfig, d discover.DI, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*WebClient, error) {
	if tlsc != nil {
		tlsc = tlsc.Clone()
	}
	if e := name.HasSelfFullName(); e != nil {
		return nil, e
	}
	serverfullname, e := name.MakeFullName(serverproject, servergroup, serverapp)
	if e != nil {
		return nil, e
	}
	if d == nil {
		return nil, errors.New("[web.client] missing discover")
	}
	if !d.CheckTarget(serverfullname) {
		return nil, errors.New("[web.client] discover's target app not match")
	}
	if c == nil {
		c = &ClientConfig{}
	}
	c.validate()

	client := &WebClient{
		serverfullname: serverfullname,
		c:              c,
		tlsc:           tlsc,
		dialer:         &net.Dialer{},

		discover: d,

		stop: graceful.New(),
	}
	p := &http.Protocols{}
	p.SetHTTP2(true)
	p.SetUnencryptedHTTP2(true)
	client.client = &http.Client{
		Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			DialContext:            client.dial,
			DialTLSContext:         client.dialtls,
			TLSClientConfig:        tlsc,
			Protocols:              p,
			MaxIdleConnsPerHost:    256,
			IdleConnTimeout:        c.IdleTimeout.StdDuration(),
			MaxResponseHeaderBytes: int64(c.MaxResponseHeader),
		},
		Timeout: c.GlobalTimeout.StdDuration(),
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Web)
	client.resolver.Start()
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
		slog.ErrorContext(ctx, "[web.client] dial failed", slog.String("sname", c.serverfullname), slog.String("sip", addr), slog.String("error", e.Error()))
	} else {
		slog.InfoContext(ctx, "[web.client] online", slog.String("sname", c.serverfullname), slog.String("sip", addr))
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
		slog.ErrorContext(ctx, "[web.client] dial failed", slog.String("sname", c.serverfullname), slog.String("sip", addr), slog.String("error", e.Error()))
		return nil, e
	}
	tmptlsc := c.tlsc.Clone()
	if tmptlsc.ServerName == "" {
		tmptlsc.ServerName = host
	}
	tc := tls.Client(conn, tmptlsc)
	if e = tc.HandshakeContext(ctx); e != nil {
		slog.ErrorContext(ctx, "[web.client] tls handshake failed", slog.String("sname", c.serverfullname), slog.String("sip", addr), slog.String("error", e.Error()))
		return nil, e
	} else {
		slog.InfoContext(ctx, "[web.client] online", slog.String("sname", c.serverfullname), slog.String("sip", addr))
	}
	return tc, nil
}
func (c *WebClient) ResolveNow() {
	go c.resolver.Now()
}

// get the server's addrs from the discover.DI(the param in NewCrpcClient)
// version can be int64 or string(should only be used with == or !=)
func (c *WebClient) GetServerIps() (ips []string, version any, lasterror error) {
	tmp, version, e := c.discover.GetAddrs(discover.NotNeed)
	ips = make([]string, 0, len(tmp))
	for k := range tmp {
		ips = append(ips, k)
	}
	lasterror = e
	return
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
	if header.Get("Core-Self") != "" {
		return true
	}
	if header.Get("Core-Deadline") != "" {
		return true
	}
	if header.Get("Core-Metadata") != "" {
		return true
	}
	if header.Get("Traceparent") != "" {
		return true
	}
	if header.Get("Tracestate") != "" {
		return true
	}
	return false
}

type forceaddrkey struct{}

// forceaddr: most of the time this should be empty
//
//	if it is not empty,this request will try to transport to this specific addr's server
//	if this specific server doesn't exist,cerror.ErrNoSpecificServer will return
//	if the DI is static:the forceaddr can be addr in the DI's addrs list
//	if the DI is dns:the forceaddr can be addr in the dns resolve result
//	if the DI is kubernetes:the forceaddr can be addr in the endpoints
func WithForceAddr(ctx context.Context, forceaddr string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, forceaddrkey{}, forceaddr)
}

// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
func (c *WebClient) Get(ctx context.Context, path, query string, header http.Header, metadata map[string]string) (resp *http.Response, e error) {
	return c.call(http.MethodGet, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
func (c *WebClient) Delete(ctx context.Context, path, query string, header http.Header, metadata map[string]string) (resp *http.Response, e error) {
	return c.call(http.MethodDelete, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
func (c *WebClient) Post(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewReader(body))
	}
	return c.call(http.MethodPost, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
func (c *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewReader(body))
	}
	return c.call(http.MethodPut, ctx, path, query, header, metadata, nil)
}

// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
func (c *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) (resp *http.Response, e error) {
	if len(body) != 0 {
		return c.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewReader(body))
	}
	return c.call(http.MethodPatch, ctx, path, query, header, metadata, nil)
}

func (c *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body io.Reader) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, cerror.ErrReq
	}
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return nil, cerror.ErrClientClosing
		}
		return nil, cerror.ErrBusy
	}
	defer c.stop.DoneOne()

	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	if query != "" && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core-Target", c.serverfullname)
	header.Set("Core-Self", name.GetSelfFullName())
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core-Metadata", common.BTS(d))
	}
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
	for {
		tctx, span := otel.Tracer("Corelib.web.client", trace.WithInstrumentationVersion(version.String())).Start(
			ctx,
			"call web",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("url.path", path), attribute.String("server.name", c.serverfullname)))
		otel.GetTextMapPropagator().Inject(tctx, propagation.HeaderCarrier(header))
		//pick server
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if ros, ok := span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				c.recordmetric(path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, true)
			}
			return nil, e
		}
		span.SetAttributes(attribute.String("server.addr", server.addr))
		var req *http.Request
		if c.tlsc != nil {
			req, e = http.NewRequestWithContext(ctx, method, "https://"+server.addr+path+query, body)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, "http://"+server.addr+path+query, body)
		}
		if e != nil {
			e = cerror.Convert(e.(*url.Error).Unwrap())
			server.GetServerPickInfo().Done(false)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if ros, ok := span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				c.recordmetric(path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, true)
			}
			return nil, e
		}
		req.Header = header
		//start call
		var resp *http.Response
		resp, e = c.client.Do(req)
		if e != nil {
			e = cerror.Convert(e.(*url.Error).Unwrap())
			server.GetServerPickInfo().Done(false)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if ros, ok := span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				c.recordmetric(path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, true)
			}
			return nil, e
		}
		if cpuusagestr := resp.Header.Get("Cpu-Usage"); cpuusagestr != "" {
			cpuusage, _ := strconv.ParseFloat(cpuusagestr, 64)
			server.GetServerPickInfo().UpdateCPU(cpuusage)
			resp.Header.Del("Cpu-Usage")
		}
		if resp.StatusCode/100 != 2 {
			var respbody []byte
			respbody, e = io.ReadAll(resp.Body)
			resp.Body.Close()
			if e != nil {
				e = cerror.Convert(e)
			} else if len(respbody) == 0 {
				e = cerror.MakeCError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				ee := cerror.Decode(common.BTS(respbody))
				ee.SetHttpcode(int32(resp.StatusCode))
				e = ee
			}
			server.GetServerPickInfo().Done(false)
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrTarget) {
				if !server.closing.Swap(true) {
					//set the lowest pick priority
					server.Pickinfo.SetDiscoverServerOffline(0)
					//rebuild picker
					c.balancer.rebuildpicker()
					//triger discover
					c.resolver.Now()
				}
				continue
			}
			if ros, ok := span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				c.recordmetric(path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, true)
			}
			return nil, e
		}
		resp.Body = &wrappedbody{c: c, path: path, body: resp.Body, span: span}
		return resp, e
	}
}

type wrappedbody struct {
	c    *WebClient
	path string
	span trace.Span
	body io.ReadCloser
}

func (b *wrappedbody) Read(p []byte) (n int, err error) {
	n, e := b.body.Read(p)
	if e != nil {
		if e == io.EOF {
			b.span.SetStatus(codes.Ok, "")
			b.span.End()
			if ros, ok := b.span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				b.c.recordmetric(b.path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, false)
			}
		} else {
			e = cerror.Convert(e)
			b.span.SetStatus(codes.Error, e.Error())
			b.span.End()
			if ros, ok := b.span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
				b.c.recordmetric(b.path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, true)
			}
		}
	}
	return n, e
}
func (b *wrappedbody) Close() error {
	if b.span.IsRecording() {
		b.span.SetStatus(codes.Ok, "")
		b.span.End()
		if ros, ok := b.span.(sdktrace.ReadOnlySpan); ok && cotel.NeedMetric() {
			b.c.recordmetric(b.path, float64(ros.EndTime().UnixNano()-ros.StartTime().UnixNano())/1000000.0, false)
		}
	}
	return b.body.Close()
}

func (c *WebClient) recordmetric(path string, usetimems float64, err bool) {
	mstatus, _ := otel.Meter("Corelib.web.client", metric.WithInstrumentationVersion(version.String())).Int64Histogram(path+".status", metric.WithUnit("1"), metric.WithExplicitBucketBoundaries(0))
	if err {
		mstatus.Record(context.Background(), 1)
	} else {
		mstatus.Record(context.Background(), 0)
	}
	mtime, _ := otel.Meter("Corelib.web.client", metric.WithInstrumentationVersion(version.String())).Float64Histogram(path+".time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	mtime.Record(context.Background(), usetimems)
}
