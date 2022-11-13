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
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"
)

type ClientConfig struct {
	ConnectTimeout time.Duration
	GlobalTimeout  time.Duration //request's max handling time
	HeartProbe     time.Duration //tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,means useless,connection will keep alive until it is closed
	IdleTimeout   time.Duration
	MaxHeader     uint
	SkipVerifyTLS bool     //don't verify the server's cert
	CAs           []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
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
}

type WebClient struct {
	selfappname   string
	serverappname string
	host          string
	globaltimeout time.Duration
	httpclient    *http.Client
	stop          *graceful.Graceful
}
type hostinfo struct {
	serverhost string
	u          *url.URL
}

// serverhost format [http/https]://[username[:password]@]the.host.name[:port]
func NewWebClient(c *ClientConfig, selfgroup, selfname, servergroup, servername, serverhost string) (*WebClient, error) {
	serverappname := servergroup + "." + servername
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if serverhost != "" {
		if u, e := url.Parse(serverhost); e != nil ||
			(u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" ||
			u.Path != "" ||
			u.RawPath != "" ||
			u.Opaque != "" ||
			u.ForceQuery ||
			u.RawQuery != "" ||
			u.Fragment != "" ||
			u.RawFragment != "" {
			return nil, errors.New("[web.client] host format wrong,should be [http/https]://[username[:password]@]the.host.name[:port]")
		}
	}
	if c == nil {
		c = &ClientConfig{}
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
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   c.ConnectTimeout,
			KeepAlive: c.HeartProbe,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},
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
		selfappname:   selfappname,
		serverappname: serverappname,
		host:          serverhost,
		globaltimeout: c.GlobalTimeout,
		httpclient: &http.Client{
			Transport: transport,
			Timeout:   c.GlobalTimeout,
		},
		stop: graceful.New(),
	}
	return client, nil
}

// serverhost format [http/https]://[username[:password]@]the.host.name[:port]
func (c *WebClient) UpdateServerHost(serverhost string) error {
	if serverhost != "" {
		if u, e := url.Parse(serverhost); e != nil ||
			(u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" ||
			u.Path != "" ||
			u.RawPath != "" ||
			u.Opaque != "" ||
			u.ForceQuery ||
			u.RawQuery != "" ||
			u.Fragment != "" ||
			u.RawFragment != "" {
			return errors.New("[web.client] host format wrong,should be [http/https]://[username[:password]@]the.host.name[:port]")
		}
	}
	c.host = serverhost
	return nil
}

// this will return the host in NewWebClient/UpdateServerHost function
func (c *WebClient) GetSeverHost() string {
	return c.host
}
func (c *WebClient) Close(force bool) {
	if force {
		c.httpclient.CloseIdleConnections()
	} else {
		c.stop.Close(c.httpclient.CloseIdleConnections, nil)
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

var ClientClosed = errors.New("[web.client] closed")

func (c *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body *bytes.Buffer) (*http.Response, error) {
	if forbiddenHeader(header) {
		return nil, cerror.MakeError(-1, 400, "forbidden header")
	}
	hostaddr := c.host
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		if hostaddr == "" {
			return nil, cerror.ErrReq
		}
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
	}
	if len(query) != 0 && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core-Target", c.serverappname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core-Metadata", common.Byte2str(d))
	}

	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid != "" {
		tracedata, _ := json.Marshal(map[string]string{
			"TraceID":      traceid,
			"SourceApp":    c.selfappname,
			"SourceMethod": selfmethod,
			"SourcePath":   selfpath,
			"Deep":         strconv.Itoa(selfdeep),
		})
		header.Set("Core-Tracedata", common.Byte2str(tracedata))
	}
	if c.globaltimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.globaltimeout))
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok {
		header.Set("Core-Deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	header.Del("Origin")
	if !c.stop.AddOne() {
		return nil, ClientClosed
	}
	defer c.stop.DoneOne()
	for {
		start := time.Now()
		if ok && dl.UnixNano() < start.UnixNano()+int64(5*time.Millisecond) {
			//at least 5ms for net lag and server logic
			return nil, cerror.ErrDeadlineExceeded
		}
		var req *http.Request
		var e error
		if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
			if body == nil {
				//io.Reader is an interface,body is *bytes.Buffer,direct pass will make the interface's value is nil but type is not nil
				req, e = http.NewRequestWithContext(ctx, method, hostaddr+path+query, nil)
			} else {
				req, e = http.NewRequestWithContext(ctx, method, hostaddr+path+query, body)
			}
		} else {
			if body == nil {
				//io.Reader is an interface,body is *bytes.Buffer,direct pass will make the interface's value is nil but type is not nil
				req, e = http.NewRequestWithContext(ctx, method, path+query, nil)
			} else {
				req, e = http.NewRequestWithContext(ctx, method, path+query, body)
			}
		}
		if e != nil {
			e = cerror.ConvertStdError(e.(*url.Error).Unwrap())
			return nil, e
		}
		req.Header = header
		//start call
		resp, e := c.httpclient.Do(req)
		end := time.Now()
		if e != nil {
			e = cerror.ConvertStdError(e.(*url.Error).Unwrap())
			log.Trace(ctx, log.CLIENT, c.serverappname, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		if resp.StatusCode/100 != 2 {
			respbody, e := io.ReadAll(resp.Body)
			resp.Body.Close()
			if e != nil {
				e = cerror.ConvertStdError(e)
				log.Trace(ctx, log.CLIENT, c.serverappname, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
				monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
				return nil, e
			}
			if len(respbody) == 0 {
				e = cerror.MakeError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				tmpe := cerror.ConvertErrorstr(common.Byte2str(respbody))
				tmpe.SetHttpcode(int32(resp.StatusCode))
				e = tmpe
			}
			if resp.StatusCode == int(cerror.ErrClosing.Httpcode) && cerror.Equal(e, cerror.ErrClosing) {
				log.Trace(ctx, log.CLIENT, c.serverappname, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, cerror.ErrClosing)
				monitor.WebClientMonitor(c.serverappname, method, path, cerror.ErrClosing, uint64(end.UnixNano()-start.UnixNano()))
				continue
			}
			log.Trace(ctx, log.CLIENT, c.serverappname, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		log.Trace(ctx, log.CLIENT, c.serverappname, req.URL.Scheme+"://"+req.URL.Host, method, path, &start, &end, nil)
		monitor.WebClientMonitor(c.serverappname, method, path, nil, uint64(end.UnixNano()-start.UnixNano()))
		return resp, nil
	}
}
