package web

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/container/trie"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	//mode 0:must have no active requests and must wait at lease WaitCloseTime
	//	every new request come in when the server is closing will refresh the WaitCloseTime
	//mode 1:must have no active requests and must wait at lease WaitCloseTime
	//	WaitCloseTime will not be refreshed by new requests
	WaitCloseMode int `json:"wait_close_mode"`
	//when server close,server will wait at least this time before close
	//min 1s,default 1s
	WaitCloseTime ctime.Duration `json:"wait_close_time"`
	//the default timeout for every web call,<=0 means no timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the GlobalTimeout
	//the client's deadline will also effect the web call's final deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time and read http header time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 2048,max 65536,unit byte
	MaxRequestHeader     uint     `json:"max_request_header"`
	CorsAllowedOrigins   []string `json:"cors_allowed_origins"`
	CorsAllowedHeaders   []string `json:"cors_allowed_headers"`
	CorsExposeHeaders    []string `json:"cors_expose_headers"`
	CorsAllowCredentials bool     `json:"cors_allow_credentials"`
	//client's Options request cache time,<=0 means ignore this setting(depend on the client's default)
	CorsMaxAge ctime.Duration `json:"cors_max_age"`
	//static source files(.html .js .css...)'s root path,empty means no static source file
	SrcRootPath string `json:"src_root_path"`
}

func (c *ServerConfig) validate() {
	if c.WaitCloseTime.StdDuration() < time.Second {
		c.WaitCloseTime = ctime.Duration(time.Second)
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(3 * time.Second)
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
	}
	if c.MaxRequestHeader < 2048 {
		c.MaxRequestHeader = 2048
	} else if c.MaxRequestHeader > 65536 {
		c.MaxRequestHeader = 65536
	}
	//allow origin
	if len(c.CorsAllowedOrigins) > 0 {
		undup := make(map[string]*struct{}, len(c.CorsAllowedOrigins))
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
			undup[v] = nil
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
		undup := make(map[string]*struct{}, len(c.CorsAllowedHeaders))
		for _, v := range c.CorsAllowedHeaders {
			if v == "*" && !c.CorsAllowCredentials {
				c.CorsAllowedHeaders = []string{"*"}
				undup = nil
				break
			} else if v == "*" {
				slog.Warn("[web.server] when cors_allow_credentials is true in config,the wildcard '*' in cors_allowed_headers is treated as the literal header name '*',without special semantics")
			}
			undup[http.CanonicalHeaderKey(v)] = nil
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
		undup := make(map[string]*struct{}, len(c.CorsExposeHeaders))
		for _, v := range c.CorsExposeHeaders {
			if v == "*" && !c.CorsAllowCredentials {
				c.CorsExposeHeaders = []string{"*"}
				undup = nil
				break
			} else if v == "*" {
				slog.Warn("[web.server] when cors_allow_credentials is true in config,the wildcard '*' in cors_expose_headers is treated as the literal header name '*',without special semantics")
			}
			undup[http.CanonicalHeaderKey(v)] = nil
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
	c              *ServerConfig
	tlsc           *tls.Config
	clientnum      int32 //without hijacked
	stop           *graceful.Graceful
	closetimer     *time.Timer
	s              *http.Server
	handlerTimeout map[string]map[string]time.Duration //first key method,second key path,value timeout,<=0 means no timeout
	handlerRewrite map[string]map[string]string        //first key method,second key origin url,value new url
}

type localport struct{}

// if tlsc is not nil,the tls will be actived
func NewWebServer(c *ServerConfig, tlsc *tls.Config) (*WebServer, error) {
	if e := name.HasSelfFullName(); e != nil {
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
		c:              c,
		tlsc:           tlsc,
		stop:           graceful.New(),
		closetimer:     time.NewTimer(0),
		handlerTimeout: make(map[string]map[string]time.Duration),
		handlerRewrite: make(map[string]map[string]string),
	}
	instance.s = &http.Server{
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo),
		TLSConfig:         tlsc,
		ReadHeaderTimeout: c.ConnectTimeout.StdDuration(),
		IdleTimeout:       c.IdleTimeout.StdDuration(),
		MaxHeaderBytes:    int(c.MaxRequestHeader),
		ConnState: func(c net.Conn, s http.ConnState) {
			if s == http.StateNew {
				atomic.AddInt32(&instance.clientnum, 1)
			} else if s == http.StateHijacked || s == http.StateClosed {
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
func (s *WebServer) NewRouter() *Router {
	router := &Router{
		s:          s,
		globalmids: make([]OutsideHandler, 0, 10),
		getTree:    trie.NewTrie[http.HandlerFunc](),
		postTree:   trie.NewTrie[http.HandlerFunc](),
		putTree:    trie.NewTrie[http.HandlerFunc](),
		patchTree:  trie.NewTrie[http.HandlerFunc](),
		deleteTree: trie.NewTrie[http.HandlerFunc](),
	}
	if s.c.SrcRootPath != "" {
		router.srcroot = os.DirFS(s.c.SrcRootPath)
	}
	return router
}
func (s *WebServer) SetRouter(r *Router) {
	s.s.Handler = r
	r.printPath()
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
			return ErrServerClosed
		}
	}
	return nil
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

// first key method,second key origin url,value new url
func (s *WebServer) UpdateHandlerRewrite(rewrite map[string]map[string]string) {
	//copy
	tmp := make(map[string]map[string]string)
	for method, paths := range rewrite {
		method = strings.ToUpper(method)
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			continue
		}
		for originurl, newurl := range paths {
			if _, ok := tmp[method]; !ok {
				tmp[method] = make(map[string]string)
			}
			if len(originurl) == 0 || originurl[0] != '/' {
				originurl = "/" + originurl
			}
			if len(newurl) == 0 || newurl[0] != '/' {
				newurl = "/" + newurl
			}
			tmp[method][originurl] = cleanPath(newurl)
		}
	}
	s.handlerRewrite = tmp
}
func (s *WebServer) getHandlerRewrite(oldpath, method string) (newpath string, ok bool) {
	rewrite := s.handlerRewrite
	paths, ok := rewrite[method]
	if !ok {
		return
	}
	newpath, ok = paths[oldpath]
	return
}

// first key path,second method,value timeout(if timeout <= 0 means no timeout)
func (s *WebServer) UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmp := make(map[string]map[string]time.Duration)
	for path := range timeout {
		for method, to := range timeout[path] {
			if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
				continue
			}
			if path == "" {
				continue
			}
			if path[0] != '/' {
				path = "/" + path
			}
			if _, ok := tmp[method]; !ok {
				tmp[method] = make(map[string]time.Duration)
			}
			tmp[method][path] = to.StdDuration()
		}
	}
	s.handlerTimeout = tmp
}

func (s *WebServer) getHandlerTimeout(path, method string) time.Duration {
	timeout := s.handlerTimeout
	paths, ok := timeout[method]
	if !ok {
		return s.c.GlobalTimeout.StdDuration()
	}
	t, ok := paths[path]
	if !ok {
		return s.c.GlobalTimeout.StdDuration()
	}
	return t
}
