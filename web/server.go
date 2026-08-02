package web

import (
	// "compress/gzip"
	// "io"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type OutsideHandler func(*ServerContext)

type ServerConfig struct {
	//mode 0:must have no active requests and must wait at lease WaitCloseTime
	//	every new request come in when the server is closing will refresh the WaitCloseTime
	//mode 1:must have no active requests and must wait at lease WaitCloseTime
	//	WaitCloseTime will not be refreshed by new requests
	WaitCloseMode int `json:"wait_close_mode"`
	//when server close,server will wait at least this time before close
	//min 1s,default 1s
	WaitCloseTime ctime.Duration `json:"wait_close_time"`
	//the default timeout for every web call,<=0 means no default timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the DefaultHandlerTimeout
	//the client's deadline will also effect the web call's final deadline
	DefaultHandlerTimeout ctime.Duration `json:"default_handler_timeout"`
	//time for connection establish,tls handshake and read one complete request
	//this can be used to prevent client's slow send attack
	//(send a big message,but every time send 1 byte,the connection is alive and works well,attacker can create lots of this kind of connections)
	//<=0 means no timeout
	ReadTimeout ctime.Duration `json:"read_timeout"`
	//time for waiting the next request,it will be reset when starting to read next message
	//expired without next message,the connection will be closed
	//<=0 means no timeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 2048,max 65536,unit byte
	MaxRequestHeader     uint     `json:"max_request_header"`
	CorsAllowedOrigins   []string `json:"cors_allowed_origins"` //can only support * or specific origin(can start of wildcard '*')
	CorsAllowedHeaders   []string `json:"cors_allowed_headers"`
	CorsExposeHeaders    []string `json:"cors_expose_headers"`
	CorsAllowCredentials bool     `json:"cors_allow_credentials"`
	//client's Options request cache time,<=0 means ignore this setting(depend on the client's default)
	CorsMaxAge ctime.Duration `json:"cors_max_age"`
	//static source files(.html .js .css...)'s root path,empty means no static source file
	//this path can't be the parent of the executable file's parent dir
	SrcRootPath string `json:"src_root_path"`
}

func (c *ServerConfig) validate() {
	if c.WaitCloseTime.StdDuration() < time.Second {
		c.WaitCloseTime = ctime.Duration(time.Second)
	}
	if c.DefaultHandlerTimeout < 0 {
		c.DefaultHandlerTimeout = 0
	}
	if c.IdleTimeout <= 0 {
		//if set server's IdleTimeout to 0,the server's ReadTimeout will be used as the server's IdleTimeout
		//so we need to set it to negative
		c.IdleTimeout = -1
	}
	if c.MaxRequestHeader < 2048 {
		c.MaxRequestHeader = 2048
	} else if c.MaxRequestHeader > 65536 {
		c.MaxRequestHeader = 65536
	}
	//allow origin
	if len(c.CorsAllowedOrigins) > 0 {
		undup := make(map[string]struct{}, len(c.CorsAllowedOrigins))
		for _, v := range c.CorsAllowedOrigins {
			if v == "*" {
				if c.CorsAllowCredentials {
					slog.Warn("[web.server] when cors_allow_credentials is true in config,the wildcard '*' in cors_allowed_origins will be ignored")
					continue
				} else {
					c.CorsAllowedOrigins = []string{"*"}
					undup = nil
					break
				}
			}
			u, e := url.Parse(v)
			if e != nil {
				panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid")
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid,scheme must be http or https")
			}
			si := strings.Index(u.Host, "*")
			ei := strings.LastIndex(u.Host, "*")
			if si != -1 && si != ei {
				panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid,host can only has one wildcard '*'")
			}
			if si != -1 && si != 0 {
				panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid,wildcard '*' in host must be the first char")
			}
			if si != -1 && len(u.Host) > 1 && u.Host[1] != '.' && u.Host[1] != ':' {
				panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid,char '.' must followed with wildcard '*'")
			}
			if pstr := u.Port(); pstr != "" {
				port, e := strconv.Atoi(pstr)
				if e != nil || port < 1 || port > 65535 {
					panic("[web.server] origin: " + v + " in cors_allowed_origins in config is invalid,port must be a number in [1,65535]")
				}
				//remove the default port
				if u.Scheme == "http" && port == 80 {
					u.Host = u.Host[:len(u.Host)-3]
				}
				if u.Scheme == "https" && port == 443 {
					u.Host = u.Host[:len(u.Host)-4]
				}
			}
			undup[strings.ToLower(u.Scheme)+"://"+strings.ToLower(u.Host)] = struct{}{}
		}
		if undup != nil {
			c.CorsAllowedOrigins = make([]string, 0, len(undup))
			for k := range undup {
				c.CorsAllowedOrigins = append(c.CorsAllowedOrigins, k)
			}
		}
	}
	//allow header
	if len(c.CorsAllowedHeaders) > 0 {
		undup := make(map[string]struct{}, len(c.CorsAllowedHeaders))
		for _, v := range c.CorsAllowedHeaders {
			if v == "*" && !c.CorsAllowCredentials {
				c.CorsAllowedHeaders = []string{"*"}
				undup = nil
				break
			} else if v == "*" {
				slog.Warn("[web.server] when cors_allow_credentials is true in config,the wildcard '*' in cors_allowed_headers is treated as the literal header name '*',without special semantics")
			}
			undup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
		if undup != nil {
			c.CorsAllowedHeaders = make([]string, 0, len(undup))
			for k := range undup {
				c.CorsAllowedHeaders = append(c.CorsAllowedHeaders, k)
			}
		}
	}
	//expose header
	if len(c.CorsExposeHeaders) > 0 {
		undup := make(map[string]struct{}, len(c.CorsExposeHeaders))
		for _, v := range c.CorsExposeHeaders {
			if v == "*" && !c.CorsAllowCredentials {
				c.CorsExposeHeaders = []string{"*"}
				undup = nil
				break
			} else if v == "*" {
				slog.Warn("[web.server] when cors_allow_credentials is true in config,the wildcard '*' in cors_expose_headers is treated as the literal header name '*',without special semantics")
			}
			undup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
		if undup != nil {
			c.CorsExposeHeaders = make([]string, 0, len(undup))
			for k := range undup {
				c.CorsExposeHeaders = append(c.CorsExposeHeaders, k)
			}
		}
	}
	if c.CorsMaxAge < 0 {
		c.CorsMaxAge = 0
	}
}

type WebServer struct {
	c         *ServerConfig
	tlsc      *tls.Config
	clientnum int32 //without hijacked
	stop      *graceful.Graceful
	//this is used to wait the register remove this instance
	closetimer *time.Timer
	s          *http.Server

	statusCounter metric.Int64Counter
	timeHistogram metric.Float64Histogram
	tracer        trace.Tracer
}

type localport struct{}

// if tlsc is not nil,the tls will be actived
func NewWebServer(c *ServerConfig, tlsc *tls.Config) (*WebServer, error) {
	if e := cotel.Init(); e != nil {
		return nil, e
	}
	sCounter, e := otel.Meter("Corelib.web.server", metric.WithInstrumentationVersion(version.String())).Int64Counter("web.server.path.status", metric.WithUnit("1"))
	if e != nil {
		return nil, e
	}
	tHistogram, e := otel.Meter("Corelib.web.server", metric.WithInstrumentationVersion(version.String())).Float64Histogram("web.server.path.time", metric.WithUnit("ms"), metric.WithExplicitBucketBoundaries(cotel.TimeBoundaries...))
	if e != nil {
		return nil, e
	}
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[web.server] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	//new server
	instance := &WebServer{
		c:             c,
		tlsc:          tlsc,
		stop:          graceful.New(),
		closetimer:    time.NewTimer(0),
		statusCounter: sCounter,
		timeHistogram: tHistogram,
		tracer:        otel.Tracer("Corelib.web.server", trace.WithInstrumentationVersion(version.String())),
	}
	p := &http.Protocols{}
	p.SetHTTP2(true)
	p.SetUnencryptedHTTP2(true)
	p.SetHTTP1(true)
	instance.s = &http.Server{
		ErrorLog:       slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo),
		TLSConfig:      tlsc,
		ReadTimeout:    c.ReadTimeout.StdDuration(),
		IdleTimeout:    c.IdleTimeout.StdDuration(),
		MaxHeaderBytes: int(c.MaxRequestHeader),
		Protocols:      p,
		ConnState: func(c net.Conn, s http.ConnState) {
			switch s {
			case http.StateNew:
				atomic.AddInt32(&instance.clientnum, 1)
			case http.StateHijacked:
				fallthrough
			case http.StateClosed:
				atomic.AddInt32(&instance.clientnum, -1)
			}
		},
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			localaddr := conn.LocalAddr().String()
			return context.WithValue(ctx, localport{}, localaddr[strings.LastIndex(localaddr, ":")+1:])
		},
	}
	<-instance.closetimer.C
	return instance, nil
}

var ErrSrcPathWrong = errors.New("[web.server] src root path wrong")
var ErrUncompress = errors.New("[web.server] uncompress gzip file in static root path failed")

// after NewRouter must call server.SetRouter to active this router
// don't forget to update the timeout and rewrite on the new router
func (s *WebServer) NewRouter() (*Router, error) {
	router := &Router{
		s:          s,
		globalmids: make([]OutsideHandler, 0, 10),
		tmpget:     make(map[string]*handler),
		tmppost:    make(map[string]*handler),
		tmppatch:   make(map[string]*handler),
		tmpput:     make(map[string]*handler),
		tmpdelete:  make(map[string]*handler),
	}
	if s.c.SrcRootPath != "" {
		p, _ := os.Executable()
		pp, e := filepath.Abs(s.c.SrcRootPath)
		if e != nil {
			return nil, ErrSrcPathWrong
		}
		rel, e := filepath.Rel(pp, p)
		if e != nil || !strings.HasPrefix(rel, "..") {
			return nil, ErrSrcPathWrong
		}
		for {
			info, e := os.Stat(s.c.SrcRootPath)
			if e != nil {
				if !os.IsNotExist(e) {
					return nil, e
				}
				if e = os.MkdirAll(s.c.SrcRootPath, 0755); e != nil {
					if os.IsExist(e) {
						continue
					}
					return nil, e
				}
			} else if !info.IsDir() {
				return nil, ErrSrcPathWrong
			}
			break
		}
		/*
			//we need to uncompress all gzip files in this dir and it's children dir
			//to make the client which can't support Accept-Encoding: gzip work
			if e := ungzip(s.c.SrcRootPath); e != nil {
				return nil, ErrUncompress
			}
		*/
		router.srcroot = os.DirFS(s.c.SrcRootPath)
	}
	return router, nil
}

/*
	func ungzip(dir string) error {
		finfos, e := os.ReadDir(dir)
		if e != nil {
			return e
		}
		need := make(map[string]struct{})
		for _, finfo := range finfos {
			if finfo.IsDir() {
				ungzip(finfo.Name())
			} else if strings.HasSuffix(".gz", finfo.Name()) {
				need[finfo.Name()] = struct{}{}
			} else {
				//already uncompressed
				delete(need, finfo.Name()+".gz")
			}
		}
		for fname := range need {
			f, e := os.Open(fname)
			if e != nil {
				return e
			}
			//uncompress
			reader, e := gzip.NewReader(f)
			if e != nil {
				return e
			}
			writer, e := os.OpenFile(fname[:len(fname)-3], os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
			if e != nil {
				return e
			}
			if _, e := io.Copy(writer, reader); e != nil {
				return e
			}
			if e := reader.Close(); e != nil {
				return e
			}
			if e := f.Close(); e != nil {
				return e
			}
			if e := writer.Sync(); e != nil {
				return e
			}
			if e := writer.Close(); e != nil {
				return e
			}
		}
		return nil
	}
*/

func (s *WebServer) SetRouter(r *Router) {
	if r == nil {
		panic("[web.server] router missing")
	}
	r.rebuild()
	s.s.Handler = r
}

var ErrServerClosed = errors.New("[web.server] closed")

func (s *WebServer) StartWebServer(listenaddr string) error {
	if s.s.Handler == nil {
		return errors.New("[web.server] call SetRouter() first")
	}
	l, e := net.Listen("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] listen tcp addr: " + listenaddr + " error: " + e.Error())
	}
	if s.tlsc != nil {
		e = s.s.ServeTLS(l, "", "")
	} else {
		e = s.s.Serve(l)
	}
	if e != nil {
		if e == http.ErrServerClosed {
			e = ErrServerClosed
		}
	}
	return e
}
func (s *WebServer) GetClientNum() int32 {
	return s.clientnum
}
func (s *WebServer) GetReqNum() int64 {
	return s.stop.GetNum()
}
func (s *WebServer) StopWebServer(force bool) {
	if force {
		s.s.Close()
	} else {
		s.stop.Close(nil, nil)
		//wait at least this.c.WaitCloseTime before stop the under layer socket
		s.closetimer.Reset(s.c.WaitCloseTime.StdDuration())
		<-s.closetimer.C
		s.s.Shutdown(context.Background())
	}
}
