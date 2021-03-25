package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/metadata"
	"github.com/julienschmidt/httprouter"
)

type ServerConfig struct {
	GlobalTimeout      time.Duration
	IdleTimeout        time.Duration //default 10min
	StaticFileRootPath string
	MaxHeader          uint
	SocketRBuf         uint //socket buffer
	SocketWBuf         uint //socker buffer
	Cors               *CorsConfig
}

type CorsConfig struct {
	AllowedOrigin    []string
	AllowedHeader    []string
	ExposeHeader     []string
	AllowCredentials bool
	MaxAge           time.Duration
	allorigin        bool
	allheader        bool
	headerstr        string
	exposestr        string
}

func (c *ServerConfig) validate() {
	if c.Cors == nil {
		c.Cors = &CorsConfig{
			AllowedOrigin:    []string{"*"},
			AllowedHeader:    []string{"*"},
			ExposeHeader:     nil,
			AllowCredentials: false,
			MaxAge:           time.Hour * 24,
		}
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
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
	for _, v := range c.Cors.AllowedOrigin {
		if v == "*" {
			c.Cors.allorigin = true
			break
		}
	}
	hasorigin := false
	for _, v := range c.Cors.AllowedHeader {
		if v == "*" {
			c.Cors.allheader = true
			break
		} else if v == "Origin" {
			hasorigin = true
		}
	}
	if !c.Cors.allheader && !hasorigin {
		c.Cors.AllowedHeader = append(c.Cors.AllowedHeader, "Origin")
	}
	c.Cors.headerstr = c.getHeaders()
	c.Cors.exposestr = c.getExpose()
}

func (c *ServerConfig) getHeaders() string {
	if c.Cors.allheader || len(c.Cors.AllowedHeader) == 0 {
		return ""
	}
	removedup := make(map[string]struct{}, len(c.Cors.AllowedHeader))
	for _, v := range c.Cors.AllowedHeader {
		if v != "*" {
			removedup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
	}
	unique := make([]string, len(removedup))
	index := 0
	for v := range removedup {
		unique[index] = v
		index++
	}
	return strings.Join(unique, ", ")
}
func (c *ServerConfig) getExpose() string {
	if len(c.Cors.ExposeHeader) > 0 {
		removedup := make(map[string]struct{}, len(c.Cors.ExposeHeader))
		for _, v := range c.Cors.ExposeHeader {
			removedup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
		unique := make([]string, len(removedup))
		index := 0
		for v := range removedup {
			unique[index] = v
			index++
		}
		return strings.Join(unique, ", ")
	} else {
		return ""
	}
}

type WebServer struct {
	selfappname string
	c           *ServerConfig
	s           *http.Server
	global      []OutsideHandler
	router      *httprouter.Router
	ctxpool     *sync.Pool
	status      int32 //0-created,not started 1-started 2-stoped
	stopch      chan struct{}
}

func NewWebServer(c *ServerConfig, selfgroup, selfname string) (*WebServer, error) {
	//pre check
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return nil, e
	}
	selfappname := selfgroup + "." + selfname
	if e := common.NameCheck(selfappname, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	//new server
	instance := &WebServer{
		selfappname: selfappname,
		c:           c,
		s: &http.Server{
			ReadTimeout:    c.GlobalTimeout,
			WriteTimeout:   c.GlobalTimeout,
			IdleTimeout:    c.IdleTimeout,
			MaxHeaderBytes: int(c.MaxHeader),
			ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
				(conn.(*net.TCPConn)).SetReadBuffer(int(c.SocketRBuf))
				(conn.(*net.TCPConn)).SetWriteBuffer(int(c.SocketWBuf))
				return ctx
			},
		},
		global:  make([]OutsideHandler, 0, 10),
		router:  httprouter.New(),
		ctxpool: &sync.Pool{},
		stopch:  make(chan struct{}, 1),
	}
	instance.s.Handler = instance
	instance.router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown path")
		http.Error(w,
			http.StatusText(http.StatusNotFound),
			http.StatusNotFound,
		)
	})
	instance.router.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown method")
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed,
		)
	})
	instance.router.PanicHandler = func(w http.ResponseWriter, r *http.Request, msg interface{}) {
		stack := make([]byte, 8192)
		n := runtime.Stack(stack, false)
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "panic:", msg, "\n"+common.Byte2str(stack[:n]))
		http.Error(w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError,
		)
	}
	instance.router.GlobalOPTIONS = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//for OPTIONS preflight
		defer w.WriteHeader(http.StatusNoContent)
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return
		}
		if r.Header.Get("Access-Control-Request-Method") == "" {
			return
		}
		headers := w.Header()
		headers.Add("Vary", "Origin")
		headers.Add("Vary", "Access-Control-Request-Method")
		headers.Add("Vary", "Access-Control-Request-Headers")
		headers.Set("Access-Control-Allow-Methods", headers.Get("Allow"))
		headers.Del("Allow")
		if c.Cors.allorigin {
			headers.Set("Access-Control-Allow-Origin", "*")
		} else {
			for _, v := range c.Cors.AllowedOrigin {
				if origin == v {
					headers.Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}
		if c.Cors.allheader {
			headers.Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		} else if len(c.Cors.headerstr) > 0 {
			headers.Set("Access-Control-Allow-Headers", c.Cors.headerstr)
		}
		if c.Cors.AllowCredentials {
			headers.Set("Access-Control-Allow-Credentials", "true")
		}
		if c.Cors.MaxAge != 0 {
			headers.Set("Access-Control-Max-Age", strconv.FormatInt(int64(c.Cors.MaxAge)/int64(time.Second), 10))
		}
	})
	if c.StaticFileRootPath != "" {
		instance.router.ServeFiles("/src/*filepath", http.Dir(c.StaticFileRootPath))
	}
	return instance, nil
}

//certkeys mapkey: cert path,mapvalue: key path
func (this *WebServer) StartWebServer(listenaddr string, certkeys map[string]string) error {
	if !atomic.CompareAndSwapInt32(&this.status, 0, 1) {
		return nil
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	certificates := make([]tls.Certificate, 0, len(certkeys))
	for cert, key := range certkeys {
		if cert != "" && key != "" {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return errors.New("[web.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
	}
	if len(certificates) > 0 {
		this.s.TLSConfig = &tls.Config{Certificates: certificates}
		e = this.s.ServeTLS(l, "", "")
	} else {
		e = this.s.Serve(l)
	}
	if e != nil {
		return errors.New("[web.server] serve error:" + e.Error())
	}
	return nil
}
func (this *WebServer) StopWebServer() {
	if atomic.SwapInt32(&this.status, 2) == 2 {
		return
	}
	tmer := time.NewTimer(time.Second)
	for {
		select {
		case <-tmer.C:
			this.s.Shutdown(context.Background())
			return
		case <-this.stopch:
			tmer.Reset(time.Second)
			for len(tmer.C) > 0 {
				<-tmer.C
			}
		}
	}
}
func (this *WebServer) getContext(w http.ResponseWriter, r *http.Request, handlers []OutsideHandler) *Context {
	ctx, ok := this.ctxpool.Get().(*Context)
	if !ok {
		return &Context{Context: r.Context(), s: this, w: w, r: r, handlers: handlers}
	}
	ctx.w = w
	ctx.r = r
	ctx.handlers = handlers
	ctx.Context = r.Context()
	return ctx
}

func (this *WebServer) putContext(ctx *Context) {
	ctx.w = nil
	ctx.r = nil
	ctx.handlers = nil
	ctx.Context = nil
	ctx.next = 0
	this.ctxpool.Put(ctx)
}

type OutsideHandler func(*Context)

//thread unsafe
func (this *WebServer) Use(globalMids ...OutsideHandler) {
	this.global = append(this.global, globalMids...)
}

//thread unsafe
func (this *WebServer) Get(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodGet, path, h)
	return nil
}

//thread unsafe
func (this *WebServer) Delete(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodDelete, path, h)
	return nil
}

//thread unsafe
func (this *WebServer) Post(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPost, path, h)
	return nil
}

//thread unsafe
func (this *WebServer) Put(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPut, path, h)
	return nil
}

//thread unsafe
func (this *WebServer) Patch(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPatch, path, h)
	return nil
}

func (this *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//check server status
	if atomic.LoadInt32(&this.status) == 0 {
		select {
		case this.stopch <- struct{}{}:
		default:
		}
		w.WriteHeader(888)
		return
	}
	//check required target server
	if targetserver := r.Header.Get("TargetServer"); targetserver != "" && targetserver != this.selfappname {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: this is not the required targetserver:", targetserver)
		w.WriteHeader(888)
		return
	}
	this.router.ServeHTTP(w, r)
}

func (this *WebServer) insideHandler(timeout time.Duration, handlers []OutsideHandler) (http.HandlerFunc, error) {
	totalhandlers := make([]OutsideHandler, 1)
	totalhandlers = append(totalhandlers, this.global...)
	totalhandlers = append(totalhandlers, handlers...)
	if len(totalhandlers) > math.MaxInt8 {
		return nil, errors.New("[web.server] too many handlers for one single path")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		//set cors
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			headers := w.Header()
			headers.Add("Vary", "Origin")
			if this.c.Cors.allorigin {
				headers.Set("Access-Control-Allow-Origin", "*")
			} else {
				find := false
				for _, v := range this.c.Cors.AllowedOrigin {
					if origin == v {
						headers.Set("Access-Control-Allow-Origin", origin)
						find = true
						break
					}
				}
				if !find {
					log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "origin:", origin, "error: cors")
					w.WriteHeader(http.StatusForbidden)
					w.Write(common.Str2byte(http.StatusText(http.StatusForbidden)))
					return
				}
			}
			if this.c.Cors.AllowCredentials {
				headers.Set("Access-Control-Allow-Credentials", "true")
			}
			if len(this.c.Cors.exposestr) > 0 {
				headers.Set("Access-Control-Expose-Headers", this.c.Cors.exposestr)
			}
		}
		//set timeout
		basectx := r.Context()
		now := time.Now()
		var globaldl int64
		var funcdl int64
		var clientdl int64
		if this.c.GlobalTimeout != 0 {
			globaldl = now.UnixNano() + int64(this.c.GlobalTimeout)
		}
		if timeout != 0 {
			funcdl = now.UnixNano() + int64(timeout)
		}
		if temp := r.Header.Get("Deadline"); temp != "" {
			var e error
			clientdl, e = strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Deadline:", temp, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(http.StatusText(http.StatusBadRequest)))
				return
			}
		}
		min := int64(math.MaxInt64)
		if clientdl < min && clientdl != 0 {
			min = clientdl
		}
		if funcdl < min && funcdl != 0 {
			min = funcdl
		}
		if globaldl < min && globaldl != 0 {
			min = globaldl
		}
		if min != math.MaxInt64 {
			if min < now.UnixNano()+int64(time.Millisecond) {
				//min logic time,1ms
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Write(common.Str2byte(http.StatusText(http.StatusGatewayTimeout)))
				return
			}
			var cancel context.CancelFunc
			basectx, cancel = context.WithDeadline(basectx, time.Unix(0, min))
			defer cancel()
		}
		if sourceserve := r.Header.Get("SourceServer"); sourceserve != "" {
			r.Header.Set("SourceServer", sourceserve+":"+r.RemoteAddr)
		}
		if mdstr := r.Header.Get("Metadata"); mdstr != "" {
			md := make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &md); e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Metadata:", mdstr, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(http.StatusText(http.StatusBadRequest)))
				return
			}
			basectx = metadata.SetAllMetadata(basectx, md)
		}
		r = r.WithContext(basectx)
		//logic
		ctx := this.getContext(w, r, totalhandlers)
		ctx.Next()
		this.putContext(ctx)
	}, nil
}
