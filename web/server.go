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
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/metadata"
	"github.com/julienschmidt/httprouter"
)

type ServerConfig struct {
	//when server close,server will wait at least this time before close,every request will refresh the time
	//min is 1 second
	WaitCloseTime time.Duration
	//request's max handling time
	GlobalTimeout time.Duration
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,GlobalTimeout will be used as IdleTimeout
	IdleTimeout time.Duration
	//system's tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	HeartProbe         time.Duration
	StaticFileRootPath string
	MaxHeader          uint
	SocketRBuf         uint
	SocketWBuf         uint
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
	if c.WaitCloseTime < time.Second {
		c.WaitCloseTime = time.Second
	}
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
	c.Cors.headerstr = c.getCorsHeaders()
	c.Cors.exposestr = c.getCorsExpose()
}

func (c *ServerConfig) getCorsHeaders() string {
	if len(c.Cors.AllowedHeader) == 0 {
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
func (c *ServerConfig) getCorsExpose() string {
	if len(c.Cors.ExposeHeader) == 0 {
		return ""
	}
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
}

type WebServer struct {
	paths            map[string]map[string]struct{} //key method,value path
	selfappname      string
	c                *ServerConfig
	ctxpool          *sync.Pool
	global           []OutsideHandler
	router           *httprouter.Router
	s                *http.Server
	closewait        *sync.WaitGroup
	totalreqnum      int32
	refreshclosewait chan struct{}
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
		paths:            make(map[string]map[string]struct{}),
		selfappname:      selfappname,
		c:                c,
		ctxpool:          &sync.Pool{},
		global:           make([]OutsideHandler, 0, 10),
		router:           httprouter.New(),
		closewait:        &sync.WaitGroup{},
		refreshclosewait: make(chan struct{}, 1),
	}
	instance.closewait.Add(1)
	instance.router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown path")
		http.Error(w,
			ERRNOAPI.Error(),
			http.StatusNotFound,
		)
	})
	instance.router.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown method")
		http.Error(w,
			ERRNOAPI.Error(),
			http.StatusMethodNotAllowed,
		)
	})
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

func (this *WebServer) printPaths() {
	for method, paths := range this.paths {
		for path := range paths {
			switch method {
			case http.MethodGet:
				log.Info("\t", method, "      ", path)
			case http.MethodDelete:
				log.Info("\t", method, "   ", path)
			case http.MethodPost:
				log.Info("\t", method, "     ", path)
			case http.MethodPut:
				log.Info("\t", method, "      ", path)
			case http.MethodPatch:
				log.Info("\t", method, "    ", path)
			}
		}
	}
}

var ErrServerClosed = errors.New("[web.server] closed")
var ErrAlreadyStarted = errors.New("[web.server] already started")

//certkeys mapkey: cert path,mapvalue: key path
func (this *WebServer) StartWebServer(listenaddr string, certkeys map[string]string) error {
	s := &http.Server{
		ReadTimeout:    this.c.GlobalTimeout,
		WriteTimeout:   this.c.GlobalTimeout,
		IdleTimeout:    this.c.IdleTimeout,
		MaxHeaderBytes: int(this.c.MaxHeader),
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			if this.c.HeartProbe > 0 {
				(conn.(*net.TCPConn)).SetKeepAlive(true)
				(conn.(*net.TCPConn)).SetKeepAlivePeriod(this.c.HeartProbe)
			}
			(conn.(*net.TCPConn)).SetReadBuffer(int(this.c.SocketRBuf))
			(conn.(*net.TCPConn)).SetWriteBuffer(int(this.c.SocketWBuf))
			return ctx
		},
	}
	if this.c.HeartProbe < 0 {
		s.SetKeepAlivesEnabled(false)
	} else {
		s.SetKeepAlivesEnabled(true)
	}
	s.Handler = this
	if len(certkeys) > 0 {
		certificates := make([]tls.Certificate, 0, len(certkeys))
		for cert, key := range certkeys {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return errors.New("[web.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		s.TLSConfig = &tls.Config{Certificates: certificates}
	}
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&this.s)), nil, unsafe.Pointer(s)) {
		return ErrAlreadyStarted
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	this.printPaths()
	if len(certkeys) > 0 {
		e = this.s.ServeTLS(l, "", "")
	} else {
		e = this.s.Serve(l)
	}
	if e != nil {
		if e == http.ErrServerClosed {
			return ErrServerClosed
		}
		return errors.New("[web.server] serve error:" + e.Error())
	}
	return nil
}
func (this *WebServer) ReplaceWebServer(newserver *WebServer) {
	this.s.Handler = newserver
	newserver.s = this.s
	*this = *newserver
	newserver.printPaths()
}
func (this *WebServer) StopWebServer() {
	defer func() {
		this.closewait.Wait()
	}()
	stop := false
	for {
		old := this.totalreqnum
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&this.totalreqnum, old, old-math.MaxInt32) {
				stop = true
				break
			}
		} else {
			break
		}
	}
	if stop {
		tmer := time.NewTimer(this.c.WaitCloseTime)
		for {
			select {
			case <-tmer.C:
				if this.totalreqnum != -math.MaxInt32 {
					tmer.Reset(this.c.WaitCloseTime)
				} else {
					this.s.Shutdown(context.Background())
					this.closewait.Done()
					return
				}
			case <-this.refreshclosewait:
				tmer.Reset(this.c.WaitCloseTime)
				for len(tmer.C) > 0 {
					<-tmer.C
				}
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
	ctx.e = nil
	return ctx
}

func (this *WebServer) putContext(ctx *Context) {
	ctx.w = nil
	ctx.r = nil
	ctx.handlers = nil
	ctx.Context = nil
	ctx.next = 0
	ctx.e = nil
	this.ctxpool.Put(ctx)
}

type OutsideHandler func(*Context)

//thread unsafe
func (this *WebServer) Use(globalMids ...OutsideHandler) {
	this.global = append(this.global, globalMids...)
}

//thread unsafe
func (this *WebServer) Get(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(http.MethodGet, path, functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodGet, path, h)
	if _, ok := this.paths[http.MethodGet]; !ok {
		this.paths[http.MethodGet] = make(map[string]struct{})
	}
	this.paths[http.MethodGet][path] = struct{}{}
	return nil
}

//thread unsafe
func (this *WebServer) Delete(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(http.MethodDelete, path, functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodDelete, path, h)
	if _, ok := this.paths[http.MethodDelete]; !ok {
		this.paths[http.MethodDelete] = make(map[string]struct{})
	}
	this.paths[http.MethodDelete][path] = struct{}{}
	return nil
}

//thread unsafe
func (this *WebServer) Post(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(http.MethodPost, path, functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPost, path, h)
	if _, ok := this.paths[http.MethodPost]; !ok {
		this.paths[http.MethodPost] = make(map[string]struct{})
	}
	this.paths[http.MethodPost][path] = struct{}{}
	return nil
}

//thread unsafe
func (this *WebServer) Put(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(http.MethodPut, path, functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPut, path, h)
	if _, ok := this.paths[http.MethodPut]; !ok {
		this.paths[http.MethodPut] = make(map[string]struct{})
	}
	this.paths[http.MethodPut][path] = struct{}{}
	return nil
}

//thread unsafe
func (this *WebServer) Patch(path string, functimeout time.Duration, handlers ...OutsideHandler) error {
	h, e := this.insideHandler(http.MethodPatch, path, functimeout, handlers)
	if e != nil {
		return e
	}
	this.router.Handler(http.MethodPatch, path, h)
	if _, ok := this.paths[http.MethodPatch]; !ok {
		this.paths[http.MethodPatch] = make(map[string]struct{})
	}
	this.paths[http.MethodPatch][path] = struct{}{}
	return nil
}

func (this *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//check required target server
	if targetserver := r.Header.Get("TargetServer"); targetserver != "" && targetserver != this.selfappname {
		log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: this is not the required targetserver:", targetserver)
		http.Error(w, ERRCLOSING.Error(), 888)
		return
	}
	//check server status
	for {
		old := this.totalreqnum
		if old >= 0 {
			//add req num
			if atomic.CompareAndSwapInt32(&this.totalreqnum, old, old+1) {
				break
			}
		} else {
			select {
			case this.refreshclosewait <- struct{}{}:
			default:
			}
			http.Error(w, ERRCLOSING.Error(), 888)
			return
		}
	}
	this.router.ServeHTTP(w, r)
	if this.totalreqnum < 0 {
		select {
		case this.refreshclosewait <- struct{}{}:
		default:
		}
	}
	atomic.AddInt32(&this.totalreqnum, -1)
}

func (this *WebServer) insideHandler(method, path string, timeout time.Duration, handlers []OutsideHandler) (http.HandlerFunc, error) {
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
		ctx := r.Context()
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
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		sourceserver := r.Header.Get("SourceServer")
		if sourceserver != "" {
			r.Header.Set("SourceServer", sourceserver+":"+r.RemoteAddr)
		}
		if mdstr := r.Header.Get("Metadata"); mdstr != "" {
			md := make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &md); e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Metadata:", mdstr, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(http.StatusText(http.StatusBadRequest)))
				return
			}
			ctx = metadata.SetAllMetadata(ctx, md)
		}
		var traceid, fromapp, fromip, frommethod, frompath string
		var fromkind trace.KIND
		if tdstr := r.Header.Get("Tracedata"); tdstr != "" {
			td := make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(tdstr), &td); e != nil {
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: Tracedata:", tdstr, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(http.StatusText(http.StatusBadRequest)))
				return
			}
			traceid = td["Traceid"]
			frommethod = td["Method"]
			frompath = td["Path"]
			fromkind = trace.KIND(td["Kind"])
		}
		fromapp = sourceserver
		if fromapp == "" {
			fromapp = "unknown"
		}
		fromip = r.RemoteAddr[:strings.Index(r.RemoteAddr, ":")]
		if frommethod == "" {
			frommethod = "unknown"
		}
		if frompath == "" {
			frompath = "unknown"
		}
		if fromkind == "" {
			fromkind = trace.KIND("unknown")
		}
		ctx = trace.InitTrace(ctx, traceid, this.selfappname, host.Hostip, method, path, trace.WEB)
		traceid, _, _, _, _, _ = trace.GetTrace(ctx)
		clientTraceCTX := trace.InitTrace(nil, traceid, fromapp, fromip, frommethod, frompath, fromkind)
		traceend := trace.TraceStart(clientTraceCTX, trace.SERVER, this.selfappname, host.Hostip, method, path, trace.WEB)
		//logic
		r = r.WithContext(ctx)
		workctx := this.getContext(w, r, totalhandlers)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 8192)
				n := runtime.Stack(stack, false)
				log.Error("[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "panic:", e, "\n"+common.Byte2str(stack[:n]))
				http.Error(w, ERRPANIC.Error(), http.StatusInternalServerError)
				workctx.e = ERRPANIC
			}
			if traceend != nil {
				traceend(workctx.e)
			}
			this.putContext(workctx)
		}()
		workctx.Next()
	}, nil
}
