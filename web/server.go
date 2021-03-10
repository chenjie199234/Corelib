package web

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/metadata"
	"github.com/julienschmidt/httprouter"
)

type WebServer struct {
	selfname string
	c        *Config
	s        *http.Server
	global   []OutsideHandler
	router   *httprouter.Router
	ctxpool  *sync.Pool
}

type Config struct {
	Timeout            time.Duration
	StaticFileRootPath string
	MaxHeader          int
	ReadBuffer         int //socket buffer
	WriteBuffer        int //socker buffer
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

func (c *Config) validate() {
	if c == nil {
		c = &Config{}
	}
	if c.Cors == nil {
		c.Cors = &CorsConfig{
			AllowedOrigin:    []string{"*"},
			AllowedHeader:    []string{"*"},
			ExposeHeader:     nil,
			AllowCredentials: false,
			MaxAge:           time.Hour * 24,
		}
	}
	if c.MaxHeader == 0 {
		c.MaxHeader = 1024
	}
	if c.ReadBuffer == 0 {
		c.ReadBuffer = 1024
	}
	if c.WriteBuffer == 0 {
		c.WriteBuffer = 1024
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
func (c *Config) getHeaders() string {
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
func (c *Config) getExpose() string {
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

func NewWebServer(c *Config, group, name string) (*WebServer, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	appname := group + "." + name
	if e := common.NameCheck(appname, false, true, false, true); e != nil {
		return nil, e
	}
	c.validate()
	instance := &WebServer{
		selfname: appname,
		c:        c,
		global:   make([]OutsideHandler, 0, 10),
		router:   httprouter.New(),
		ctxpool:  &sync.Pool{},
	}
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
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "panic:", msg)
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
func (this *WebServer) StartWebServer(listenaddr string, cert, key string) error {
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	this.s = &http.Server{
		Handler:        this,
		ReadTimeout:    this.c.Timeout,
		MaxHeaderBytes: this.c.MaxHeader,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			(c.(*net.TCPConn)).SetReadBuffer(this.c.ReadBuffer)
			(c.(*net.TCPConn)).SetWriteBuffer(this.c.WriteBuffer)
			return ctx
		},
	}
	if cert != "" && key != "" {
		e = this.s.ServeTLS(l, cert, key)

	} else {
		this.s.Serve(l)
	}
	if e != nil {
		return errors.New("[web.server] serve error:" + e.Error())
	}
	return nil
}
func (this *WebServer) StopWebServer() {
	this.s.Shutdown(context.Background())
}
func (this *WebServer) getContext(w http.ResponseWriter, r *http.Request, handlers []OutsideHandler) *Context {
	ctx, ok := this.ctxpool.Get().(*Context)
	if !ok {
		return &Context{Context: r.Context(), w: w, r: r, handlers: handlers}
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
		if this.c.Timeout != 0 {
			globaldl = now.UnixNano() + int64(this.c.Timeout)
		}
		if timeout != 0 {
			funcdl = now.UnixNano() + int64(timeout)
		}
		if temp := r.Header.Get("Deadline"); temp != "" {
			var e error
			clientdl, e = strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Deadline", temp, "format error")
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
		if mdstr := r.Header.Get("Metadata"); mdstr != "" {
			md := make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &md); e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Metadata", mdstr, "format error")
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
