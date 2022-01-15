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
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/name"
)

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
	selfappname   string
	serverappname string
	c             *ClientConfig
	httpclient    *http.Client
}

func NewWebClient(c *ClientConfig, selfgroup, selfname, servergroup, servername string) (*WebClient, error) {
	serverappname := servergroup + "." + servername
	if e := name.FullCheck(serverappname); e != nil {
		return nil, e
	}
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
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
			Timeout:   c.ConnTimeout,
			KeepAlive: c.HeartProbe,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},
		TLSHandshakeTimeout:    c.ConnTimeout,
		ForceAttemptHTTP2:      true,
		MaxIdleConnsPerHost:    50,
		IdleConnTimeout:        c.IdleTimeout,
		MaxResponseHeaderBytes: int64(c.MaxHeader),
		ReadBufferSize:         int(c.SocketRBuf),
		WriteBufferSize:        int(c.SocketWBuf),
	}
	if c.HeartProbe < 0 {
		transport.DisableKeepAlives = true
	}
	client := &WebClient{
		selfappname:   selfappname,
		serverappname: serverappname,
		c:             c,
		httpclient: &http.Client{
			Transport: transport,
			Timeout:   c.GlobalTimeout,
		},
	}
	return client, nil
}

func forbiddenHeader(header http.Header) bool {
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

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Get(ctx context.Context, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodGet, ctx, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Delete(ctx context.Context, path, query string, header http.Header, metadata map[string]string) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	return this.call(http.MethodDelete, ctx, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Post(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPost, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPost, ctx, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Put(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPut, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPut, ctx, path, query, header, metadata, nil)
}

//"Core_deadline" "Core_target" "Core_metadata" "Core_tracedata" are forbidden in header
func (this *WebClient) Patch(ctx context.Context, path, query string, header http.Header, metadata map[string]string, body []byte) ([]byte, error) {
	if forbiddenHeader(header) {
		return nil, errors.New("[web.client] forbidden header")
	}
	if len(body) != 0 {
		return this.call(http.MethodPatch, ctx, path, query, header, metadata, bytes.NewBuffer(body))
	}
	return this.call(http.MethodPatch, ctx, path, query, header, metadata, nil)
}
func (this *WebClient) call(method string, ctx context.Context, path, query string, header http.Header, metadata map[string]string, body *bytes.Buffer) ([]byte, error) {
	parsedurl, e := url.Parse(path)
	if e != nil {
		return nil, cerror.ConvertStdError(e)
	}
	if len(query) != 0 && query[0] != '?' {
		query = "?" + query
	}
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Core_target", this.serverappname)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		header.Set("Core_metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := trace.GetTrace(ctx)
	if traceid != "" {
		header.Set("Core_tracedata", traceid)
		header.Add("Core_tracedata", this.selfappname)
		header.Add("Core_tracedata", selfmethod)
		header.Add("Core_tracedata", selfpath)
		header.Add("Core_tracedata", strconv.Itoa(selfdeep))
	}
	if this.c.GlobalTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(this.c.GlobalTimeout))
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
			return nil, cerror.ErrDeadlineExceeded
		}
		var req *http.Request
		var e error
		if body == nil {
			req, e = http.NewRequestWithContext(ctx, method, path+query, nil)
		} else {
			req, e = http.NewRequestWithContext(ctx, method, path+query, body)
		}
		if e != nil {
			return nil, cerror.ConvertStdError(e)
		}
		req.Header = header
		//start call
		resp, e := this.httpclient.Do(req)
		end := time.Now()
		if e != nil {
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, this.serverappname, parsedurl.Scheme+"://"+parsedurl.Host, method, parsedurl.Path, &start, &end, e)
			return nil, e
		}
		respbody, e := io.ReadAll(resp.Body)
		resp.Body.Close()
		if e != nil {
			e = cerror.ConvertStdError(e)
			trace.Trace(ctx, trace.CLIENT, this.serverappname, parsedurl.Scheme+"://"+parsedurl.Host, method, parsedurl.Path, &start, &end, e)
			return nil, e
		}
		if resp.StatusCode == int(cerror.ErrClosing.Httpcode) && cerror.Equal(cerror.ConvertErrorstr(common.Byte2str(respbody)), cerror.ErrClosing) {
			trace.Trace(ctx, trace.CLIENT, this.serverappname, parsedurl.Scheme+"://"+parsedurl.Host, method, parsedurl.Path, &start, &end, cerror.ErrClosing)
			continue
		} else if resp.StatusCode != http.StatusOK {
			if len(respbody) == 0 {
				e = cerror.MakeError(-1, int32(resp.StatusCode), http.StatusText(resp.StatusCode))
			} else {
				tempe := cerror.ConvertErrorstr(common.Byte2str(respbody))
				tempe.SetHttpcode(int32(resp.StatusCode))
				e = tempe
			}
			trace.Trace(ctx, trace.CLIENT, this.serverappname, parsedurl.Scheme+"://"+parsedurl.Host, method, parsedurl.Path, &start, &end, e)
			return nil, e
		}
		trace.Trace(ctx, trace.CLIENT, this.serverappname, parsedurl.Scheme+"://"+parsedurl.Host, method, parsedurl.Path, &start, &end, nil)
		return respbody, nil
	}
}
