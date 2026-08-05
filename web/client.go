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
	"strings"
	"sync/atomic"
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
	//if ctx's Deadline exist and DefaultHandlerTimeout > 0,the min(time.Now().Add(DefaultHandlerTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and DefaultHandlerTimeout > 0 ,the time.Now().Add(DefaultHandlerTimeout) will be used as the final deadline
	//if ctx's Deadline not exist and DefaultHandlerTimeout <=0,means no deadline
	DefaultHandlerTimeout ctime.Duration `json:"default_handler_timeout"`
	//time for connection establish(include dial time,tls handshake time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time
	//<=0 means no timeout
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

	statusCounter metric.Int64Counter
	timeHistogram metric.Float64Histogram
	tracer        trace.Tracer

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be activated
func NewWebClient(c *ClientConfig, d discover.DI, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*WebClient, error) {
	if e := cotel.Init(); e != nil {
		return nil, e
	}
	sCounter, e := otel.Meter("Corelib.web.client", metric.WithInstrumentationVersion(version.String())).Int64Counter("web.client.path.status", metric.WithUnit("1"))
	if e != nil {
		return nil, e
	}
	tHistogram, e := otel.Meter("Corelib.web.client", metric.WithInstrumentationVersion(version.String())).Float64Histogram("web.client.path.time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	if e != nil {
		return nil, e
	}
	if tlsc != nil {
		tlsc = tlsc.Clone()
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
		dialer:         &net.Dialer{Timeout: c.ConnectTimeout.StdDuration()},

		discover: d,

		statusCounter: sCounter,
		timeHistogram: tHistogram,
		tracer:        otel.Tracer("Corelib.web.client", trace.WithInstrumentationVersion(version.String())),

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
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Web)
	client.resolver.Start()
	return client, nil
}

// this is for http.Transport
func (c *WebClient) dial(ctx context.Context, network, addr string) (net.Conn, error) {
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
	conn, e := c.dialer.DialContext(ctx, network, addr)
	if e != nil {
		slog.ErrorContext(ctx, "[web.client] dial failed", slog.String("sname", c.serverfullname), slog.String("sip", addr), slog.String("error", e.Error()))
		return nil, e
	}
	index := strings.LastIndex(addr, ":")
	if index == -1 {
		index = len(addr)
	}
	hostname := addr[:index]
	tmptlsc := c.tlsc.Clone()
	if tmptlsc.ServerName == "" {
		tmptlsc.ServerName = hostname
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

type ErrorParser func(httpcode int, httpresp []byte) *cerror.Error

func defaultErrorParser(httpcode int, httpresp []byte) *cerror.Error {
	if len(httpresp) == 0 {
		return cerror.MakeCError(-1, int32(httpcode), http.StatusText(httpcode))
	}
	ee := cerror.Decode(common.BTS(httpresp))
	ee.SetHttpcode(int32(httpcode))
	return ee
}

// Warning! If you use this to call other's server(unreliable,not in self group),the metadata may leak data,please set it to nil
// Warning! Don't forget to call the resp.Body.Close(),even you get the io.EOF on the resp.Body
// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
// eparser is used to decode error message which has different format,it will only be called when http response code is not 2xx
func (c *WebClient) Get(ctx context.Context, path, query string, header http.Header, metadata map[string]string, eparser ErrorParser) (resp *http.Response, e error) {
	if eparser == nil {
		eparser = defaultErrorParser
	}
	return c.call(http.MethodGet, ctx, path, query, header, metadata, nil, eparser)
}

// Warning! If you use this to call other's server(unreliable,not in self group),the metadata may leak data,please set it to nil
// Warning! Don't forget to call the resp.Body.Close(),even you get the io.EOF on the resp.Body
// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
// eparser is used to decode error message which has different format,it will only be called when http response code is not 2xx
func (c *WebClient) Delete(ctx context.Context, path, query string, header http.Header, metadata map[string]string, eparser ErrorParser) (resp *http.Response, e error) {
	if eparser == nil {
		eparser = defaultErrorParser
	}
	return c.call(http.MethodDelete, ctx, path, query, header, metadata, nil, eparser)
}

// Warning! If you use this to call other's server(unreliable,not in self group),the metadata may leak data,please set it to nil
// Warning! Don't forget to call the resp.Body.Close(),even you get the io.EOF on the resp.Body
// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
// eparser is used to decode error message which has different format,it will only be called when http response code is not 2xx
func (c *WebClient) Post(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte, eparser ErrorParser) (resp *http.Response, e error) {
	if eparser == nil {
		eparser = defaultErrorParser
	}
	if len(body) != 0 {
		return c.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewReader(body), eparser)
	}
	return c.call(http.MethodPost, ctx, path, query, header, metadata, nil, eparser)
}

// Warning! If you use this to call other's server(unreliable,not in self group),the metadata may leak data,please set it to nil
// Warning! Don't forget to call the resp.Body.Close(),even you get the io.EOF on the resp.Body
// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
// eparser is used to decode error message which has different format,it will only be called when http response code is not 2xx
func (c *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte, eparser ErrorParser) (resp *http.Response, e error) {
	if eparser == nil {
		eparser = defaultErrorParser
	}
	if len(body) != 0 {
		return c.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewReader(body), eparser)
	}
	return c.call(http.MethodPut, ctx, path, query, header, metadata, nil, eparser)
}

// Warning! If you use this to call other's server(unreliable,not in self group),the metadata may leak data,please set it to nil
// Warning! Don't forget to call the resp.Body.Close(),even you get the io.EOF on the resp.Body
// "Core-Deadline" "Core-Target" "Core-Self" "Core-Metadata" "Traceparent" "Tracestate" are forbidden in header
// eparser is used to decode error message which has different format,it will only be called when http response code is not 2xx
func (c *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte, eparser ErrorParser) (resp *http.Response, e error) {
	if eparser == nil {
		eparser = defaultErrorParser
	}
	if len(body) != 0 {
		return c.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewReader(body), eparser)
	}
	return c.call(http.MethodPatch, ctx, path, query, header, metadata, nil, eparser)
}

func (c *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body io.Reader, eparser ErrorParser) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, cerror.ErrReq
	}
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return nil, cerror.ErrClientClosing
		}
		return nil, cerror.ErrBusy
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
	header.Set("Core-Target", c.serverfullname)
	header.Set("Core-Self", name.GetSelfFullName())
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core-Metadata", common.BTS(d))
	}
	if c.c.DefaultHandlerTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.c.DefaultHandlerTimeout.StdDuration())
		defer cancel()
	}
	if dl, ok := ctx.Deadline(); ok {
		header.Set("Core-Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	for {
		tctx, span := c.tracer.Start(ctx, "call web", trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("path", path), attribute.String("sname", c.serverfullname)))
		otel.GetTextMapPropagator().Inject(tctx, propagation.HeaderCarrier(header))
		var stime int64 //used in metric and balancer
		if cotel.NeedTrace() {
			stime = span.(sdktrace.ReadOnlySpan).StartTime().UnixNano()
		} else {
			stime = time.Now().UnixNano()
		}
		//pick server
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			c.stop.DoneOne()
			return nil, e
		}
		span.SetAttributes(attribute.String("sip", server.addr))
		var req *http.Request
		if c.tlsc != nil {
			req, e = http.NewRequestWithContext(ctx, method, "https://"+server.addr+path+query, body)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, "http://"+server.addr+path+query, body)
		}
		if e != nil {
			e = cerror.Convert(e.(*url.Error).Unwrap())
			span.SetStatus(codes.Error, e.Error())
			span.End()
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			c.stop.DoneOne()
			return nil, e
		}
		req.Header = header
		//start call
		var resp *http.Response
		resp, e = c.client.Do(req)
		if e != nil {
			e = cerror.Convert(e.(*url.Error).Unwrap())
			span.SetStatus(codes.Error, e.Error())
			span.End()
			server.GetServerPickInfo().Done(false, 0)
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			c.stop.DoneOne()
			return nil, e
		}
		if resp.StatusCode/100 != 2 {
			var respbody []byte
			respbody, e = io.ReadAll(resp.Body)
			resp.Body.Close()
			if e != nil {
				e = cerror.Convert(e)
			} else {
				e = eparser(resp.StatusCode, respbody)
			}
			span.SetStatus(codes.Error, e.Error())
			span.End()
			var etime int64
			if cotel.NeedTrace() {
				etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
			} else {
				etime = time.Now().UnixNano()
			}
			server.GetServerPickInfo().Done(false, uint64(etime-stime))
			if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrTarget) {
				//the server will not handle this call,we can retry this request
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
			//only record the real call's metric
			if cotel.NeedMetric() {
				c.recordmetric(path, float64(etime-stime)/1000000.0, true)
			}
			c.stop.DoneOne()
			return nil, e
		}
		resp.Body = &wrappedbody{s: server, c: c, stime: stime, path: path, body: resp.Body, span: span}
		return resp, e
	}
}

type wrappedbody struct {
	s       *ServerForPick
	c       *WebClient
	stime   int64
	path    string
	span    trace.Span
	body    io.ReadCloser
	cleaned atomic.Bool
}

func (b *wrappedbody) Read(p []byte) (n int, err error) {
	n, e := b.body.Read(p)
	if e != nil {
		b.clean(e)
	}
	return n, e
}
func (b *wrappedbody) Close() error {
	b.clean(nil)
	return b.body.Close()
}
func (b *wrappedbody) clean(e error) {
	if b.cleaned.Swap(true) {
		return
	}
	if e == nil || e == io.EOF {
		b.span.SetStatus(codes.Ok, "")
	} else {
		e = cerror.Convert(e)
		b.span.SetStatus(codes.Error, e.Error())
	}
	b.span.End()
	var etime int64
	if cotel.NeedTrace() {
		etime = b.span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
	} else {
		etime = time.Now().UnixNano()
	}
	b.s.GetServerPickInfo().Done(e == nil, uint64(etime-b.stime))
	if cotel.NeedMetric() {
		b.c.recordmetric(b.path, float64(etime-b.stime)/1000000.0, false)
	}
	b.c.stop.DoneOne()
}

func (c *WebClient) recordmetric(path string, usetimems float64, err bool) {
	attr := attribute.String("path", path)
	if err {
		c.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "error")))
	} else {
		c.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "ok")))
	}
	c.timeHistogram.Record(context.Background(), usetimems, metric.WithAttributes(attr))
}
