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

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
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
	serverhost    string
	u             *url.URL
	c             *ClientConfig
	httpclient    *http.Client
}

//serverhost format [http/https]://[username[:password]@]the.host.name[:port]
func NewWebClient(c *ClientConfig, selfgroup, selfname, servergroup, servername, serverhost string) (*WebClient, error) {
	serverappname := servergroup + "." + servername
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	var u *url.URL
	if serverhost != "" {
		var e error
		u, e = url.Parse(serverhost)
		if e != nil ||
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
		MaxIdleConnsPerHost:    50,
		IdleConnTimeout:        c.IdleTimeout,
		MaxResponseHeaderBytes: int64(c.MaxHeader),
	}
	if c.HeartProbe < 0 {
		transport.DisableKeepAlives = true
	}
	client := &WebClient{
		selfappname:   selfappname,
		serverappname: serverappname,
		serverhost:    serverhost,
		u:             u,
		c:             c,
		httpclient: &http.Client{
			Transport: transport,
			Timeout:   c.GlobalTimeout,
		},
	}
	return client, nil
}
func (c *WebClient) GetSeverHost() string {
	return c.u.Scheme + "://" + c.u.Host
}

func forbiddenHeader(header http.Header) bool {
	if header == nil {
		return false
	}
	if _, ok := header["Core_target"]; ok {
		return true
	}
	if _, ok := header["Core_deadline"]; ok {
		return true
	}
	if _, ok := header["Core_metadata"]; ok {
		return true
	}
	if _, ok := header["Core_tracedata"]; ok {
		return true
	}
	return false
}

//if path start with [http/https]://[username[:password]@]the.host.name[:port],the serverhost setted when this client was created will be ignored
//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (c *WebClient) Get(ctx context.Context, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	return c.call(http.MethodGet, ctx, path, query, header, metadata, nil)
}

//if path start with [http/https]://[username[:password]@]the.host.name[:port],the serverhost setted when this client was created will be ignored
//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (c *WebClient) Delete(ctx context.Context, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	return c.call(http.MethodDelete, ctx, path, query, header, metadata, nil)
}

//if path start with [http/https]://[username[:password]@]the.host.name[:port],the serverhost setted when this client was created will be ignored
//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (c *WebClient) Post(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if len(body) != 0 {
		return c.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPost, ctx, path, query, header, metadata, nil)
}

//if path start with [http/https]://[username[:password]@]the.host.name[:port],the serverhost setted when this client was created will be ignored
//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (c *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if len(body) != 0 {
		return c.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPut, ctx, path, query, header, metadata, nil)
}

//if path start with [http/https]://[username[:password]@]the.host.name[:port],the serverhost setted when this client was created will be ignored
//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (c *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if len(body) != 0 {
		return c.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return c.call(http.MethodPatch, ctx, path, query, header, metadata, nil)
}
func (c *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body *bytes.Buffer) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, cerror.MakeError(-1, 400, "forbidden header")
	}
	var u *url.URL
	if !strings.HasPrefix(path, "https") && !strings.HasPrefix(path, "http") {
		if c.serverhost == "" {
			return nil, cerror.ErrReq
		}
		u = c.u
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
	} else {
		var e error
		u, e = url.Parse(path)
		if e != nil {
			e = cerror.ConvertStdError(e)
			return nil, e
		}
	}
	if len(query) != 0 && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core_target", c.serverappname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core_metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := trace.GetTrace(ctx)
	if traceid != "" {
		header.Set("Core_tracedata", traceid)
		header.Add("Core_tracedata", c.selfappname)
		header.Add("Core_tracedata", selfmethod)
		header.Add("Core_tracedata", selfpath)
		header.Add("Core_tracedata", strconv.Itoa(selfdeep))
	}
	if c.c.GlobalTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok {
		header.Set("Core_deadline", strconv.FormatInt(dl.UnixNano(), 10))
	}
	header.Del("Origin")
	for {
		start := time.Now()
		if ok && dl.UnixNano() < start.UnixNano()+int64(5*time.Millisecond) {
			//at least 5ms for net lag and server logic
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.selfappname, u.Scheme+"://"+u.Host, method, path, &start, &end, cerror.ErrDeadlineExceeded)
			monitor.WebClientMonitor(c.serverappname, method, path, cerror.ErrDeadlineExceeded, uint64(end.UnixNano()-start.UnixNano()))
			return nil, cerror.ErrDeadlineExceeded
		}
		var req *http.Request
		var e error
		if strings.HasPrefix(path, "https") || strings.HasPrefix(path, "http") {
			if body == nil {
				//io.Reader is an interface,body is *bytes.Buffer,direct pass will make the interface's value is nil but type is not nil
				req, e = http.NewRequestWithContext(ctx, method, path+query, nil)
			} else {
				req, e = http.NewRequestWithContext(ctx, method, path+query, body)
			}
		} else {
			if body == nil {
				//io.Reader is an interface,body is *bytes.Buffer,direct pass will make the interface's value is nil but type is not nil
				req, e = http.NewRequestWithContext(ctx, method, c.serverhost+query, nil)
			} else {
				req, e = http.NewRequestWithContext(ctx, method, c.serverhost+query, body)
			}
		}
		if e != nil {
			e = cerror.ConvertStdError(e)
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.selfappname, u.Scheme+"://"+u.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		req.Header = header
		//start call
		resp, e := c.httpclient.Do(req)
		end := time.Now()
		if e != nil {
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, c.serverappname, u.Scheme+"://"+u.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		respbody, e := io.ReadAll(resp.Body)
		resp.Body.Close()
		if e != nil {
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, c.serverappname, u.Scheme+"://"+u.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		if resp.StatusCode == int(cerror.ErrClosing.Httpcode) && cerror.Equal(cerror.ConvertErrorstr(common.Byte2str(respbody)), cerror.ErrClosing) {
			trace.Trace(ctx, trace.CLIENT, c.serverappname, u.Scheme+"://"+u.Host, method, path, &start, &end, cerror.ErrClosing)
			monitor.WebClientMonitor(c.serverappname, method, path, cerror.ErrClosing, uint64(end.UnixNano()-start.UnixNano()))
			continue
		} else if resp.StatusCode != http.StatusOK {
			if len(respbody) == 0 {
				e = cerror.MakeError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				tempe := cerror.ConvertErrorstr(common.Byte2str(respbody))
				tempe.SetHttpcode(int32(resp.StatusCode))
				e = tempe
			}
			trace.Trace(ctx, trace.CLIENT, c.serverappname, u.Scheme+"://"+u.Host, method, path, &start, &end, e)
			monitor.WebClientMonitor(c.serverappname, method, path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		trace.Trace(ctx, trace.CLIENT, c.serverappname, u.Scheme+"://"+u.Host, method, path, &start, &end, nil)
		monitor.WebClientMonitor(c.serverappname, method, path, nil, uint64(end.UnixNano()-start.UnixNano()))
		return respbody, nil
	}
}
