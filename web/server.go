package web

import (
	"context"
	"crypto/tls"
	"encoding/base64"
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

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/julienschmidt/httprouter"
)

type ServerConfig struct {
	//when server close,server will wait at least this time before close,every request will refresh the time
	//min is 1 second
	WaitCloseTime time.Duration
	//request's max handling time(including connection establish time)
	GlobalTimeout time.Duration
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,GlobalTimeout will be used as IdleTimeout
	IdleTimeout        time.Duration
	HeartProbe         time.Duration //system's tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	StaticFileRootPath string
	MaxHeader          uint
	SocketRBuf         uint
	SocketWBuf         uint
	CertKeys           map[string]string //mapkey: cert path,mapvalue: key path
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
	allpaths         map[string]map[string]struct{} //key method,second key path
	selfappname      string
	c                *ServerConfig
	ctxpool          *sync.Pool
	global           []OutsideHandler
	router           *httprouter.Router
	s                *http.Server
	closewait        *sync.WaitGroup
	totalreqnum      int32
	refreshclosewait chan *struct{}
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
		allpaths:    make(map[string]map[string]struct{}),
		selfappname: selfappname,
		c:           c,
		ctxpool:     &sync.Pool{},
		global:      make([]OutsideHandler, 0, 10),
		router:      httprouter.New(),
		s: &http.Server{
			ReadTimeout:    c.GlobalTimeout,
			WriteTimeout:   c.GlobalTimeout,
			IdleTimeout:    c.IdleTimeout,
			MaxHeaderBytes: int(c.MaxHeader),
			ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
				if c.HeartProbe > 0 {
					(conn.(*net.TCPConn)).SetKeepAlive(true)
					(conn.(*net.TCPConn)).SetKeepAlivePeriod(c.HeartProbe)
				}
				(conn.(*net.TCPConn)).SetReadBuffer(int(c.SocketRBuf))
				(conn.(*net.TCPConn)).SetWriteBuffer(int(c.SocketWBuf))
				return ctx
			},
		},
		closewait:        &sync.WaitGroup{},
		refreshclosewait: make(chan *struct{}, 1),
	}
	if len(c.CertKeys) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.CertKeys))
		for cert, key := range c.CertKeys {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[web.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		instance.s.TLSConfig = &tls.Config{Certificates: certificates}
	}
	instance.s.Handler = instance
	if c.HeartProbe < 0 {
		instance.s.SetKeepAlivesEnabled(false)
	} else {
		instance.s.SetKeepAlivesEnabled(true)
	}
	instance.closewait.Add(1)
	instance.router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error(r.Context(), "[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown path")
		w.WriteHeader(http.StatusNotImplemented)
		w.Header().Set("Content-Type", "application/json")
		w.Write(common.Str2byte(_ErrNoapiStr))
	})
	instance.router.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error(r.Context(), "[web.server] client ip:", getclientip(r), "path:", r.URL.Path, "method:", r.Method, "error: unknown method")
		w.WriteHeader(http.StatusNotImplemented)
		w.Header().Set("Content-Type", "application/json")
		w.Write(common.Str2byte(_ErrNoapiStr))
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
	for method, paths := range this.allpaths {
		for path := range paths {
			switch method {
			case http.MethodGet:
				log.Info(nil, "\t", method, "      ", path)
			case http.MethodDelete:
				log.Info(nil, "\t", method, "   ", path)
			case http.MethodPost:
				log.Info(nil, "\t", method, "     ", path)
			case http.MethodPut:
				log.Info(nil, "\t", method, "      ", path)
			case http.MethodPatch:
				log.Info(nil, "\t", method, "    ", path)
			}
		}
	}
}

var ErrServerClosed = errors.New("[web.server] closed")

func (this *WebServer) StartWebServer(listenaddr string) error {
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	this.printPaths()
	if len(this.c.CertKeys) > 0 {
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
	defer this.closewait.Wait()
	stop := false
	for {
		old := atomic.LoadInt32(&this.totalreqnum)
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
				if atomic.LoadInt32(&this.totalreqnum) != -math.MaxInt32 {
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

//type HandlerTimeoutConfig struct {
//        Method  string //GET,POST,PUT,PATCH,DELETE,GRPC,CRPC
//        Path    string
//        Timeout time.Duration //0 means no handler specific timeout,but still has global timeout
//}

//func (this *WebServer) UpdateHandlerTimeout(htcs []*HandlerTimeoutConfig) error {
//        tmp := make(map[string]map[string]time.Duration)
//        for _, htc := range htcs {
//                if htc.Timeout == 0 {
//                        //jump,0 means no handler specific timeout
//                        continue
//                }
//                method := strings.ToUpper(htc.Method)
//                if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
//                        return errors.New("[web.server.UpdateHandlerTimeout] unknown method")
//                }
//                if _, ok := tmp[method]; !ok {
//                        tmp[method] = make(map[string]time.Duration)
//                }
//                var path string
//                if len(htc.Path) == 0 || htc.Path[0] != '/' {
//                        path = "/" + htc.Path
//                }
//                tmp[method][path] = htc.Timeout
//        }
//        atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout)), unsafe.Pointer(&tmp))
//        return nil
//}
//func (this *WebServer) getHandlerTimeout(method, path string) time.Duration {
//        handlerTimeout := *(*map[string]map[string]time.Duration)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout))))
//        if m, ok := handlerTimeout[method]; ok {
//                if t, ok := m[path]; ok {
//                        return t
//                }
//        }
//        return 0
//}

type OutsideHandler func(*Context)

//thread unsafe
func (this *WebServer) Use(globalMids ...OutsideHandler) {
	this.global = append(this.global, globalMids...)
}

//thread unsafe
func (this *WebServer) Get(path string, handlers ...OutsideHandler) {
	if len(path) == 0 || path[0] != '/' {
		panic("[web.server] path must start with /")
	}
	this.router.Handler(http.MethodGet, path, this.insideHandler(http.MethodGet, path, handlers))
	if _, ok := this.allpaths[http.MethodGet]; !ok {
		this.allpaths[http.MethodGet] = make(map[string]struct{})
	}
	this.allpaths[http.MethodGet][path] = struct{}{}
}

//thread unsafe
func (this *WebServer) Delete(path string, handlers ...OutsideHandler) {
	if len(path) == 0 || path[0] != '/' {
		panic("[web.server] path must start with /")
	}
	this.router.Handler(http.MethodDelete, path, this.insideHandler(http.MethodDelete, path, handlers))
	if _, ok := this.allpaths[http.MethodDelete]; !ok {
		this.allpaths[http.MethodDelete] = make(map[string]struct{})
	}
	this.allpaths[http.MethodDelete][path] = struct{}{}
	return
}

//thread unsafe
func (this *WebServer) Post(path string, handlers ...OutsideHandler) {
	if len(path) == 0 || path[0] != '/' {
		panic("[web.server] path must start with /")
	}
	this.router.Handler(http.MethodPost, path, this.insideHandler(http.MethodPost, path, handlers))
	if _, ok := this.allpaths[http.MethodPost]; !ok {
		this.allpaths[http.MethodPost] = make(map[string]struct{})
	}
	this.allpaths[http.MethodPost][path] = struct{}{}
	return
}

//thread unsafe
func (this *WebServer) Put(path string, handlers ...OutsideHandler) {
	if len(path) == 0 || path[0] != '/' {
		panic("[web.server] path must start with /")
	}
	this.router.Handler(http.MethodPut, path, this.insideHandler(http.MethodPut, path, handlers))
	if _, ok := this.allpaths[http.MethodPut]; !ok {
		this.allpaths[http.MethodPut] = make(map[string]struct{})
	}
	this.allpaths[http.MethodPut][path] = struct{}{}
	return
}

//thread unsafe
func (this *WebServer) Patch(path string, handlers ...OutsideHandler) {
	if len(path) == 0 || path[0] != '/' {
		panic("[web.server] path must start with /")
	}
	this.router.Handler(http.MethodPatch, path, this.insideHandler(http.MethodPatch, path, handlers))
	if _, ok := this.allpaths[http.MethodPatch]; !ok {
		this.allpaths[http.MethodPatch] = make(map[string]struct{})
	}
	this.allpaths[http.MethodPatch][path] = struct{}{}
	return
}

func (this *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if target := r.Header.Get("Core_target"); target != "" && target != this.selfappname {
		//this is not the required server.tell peer self closed
		w.WriteHeader(888)
		w.Header().Set("Content-Type", "application/json")
		w.Write(common.Str2byte(_ErrClosingStr))
		return
	}
	//check server status
	for {
		old := atomic.LoadInt32(&this.totalreqnum)
		if old >= 0 {
			//add req num
			if atomic.CompareAndSwapInt32(&this.totalreqnum, old, old+1) {
				break
			}
		} else {
			//refresh close wait
			select {
			case this.refreshclosewait <- nil:
			default:
			}
			//tell peer self closed
			w.WriteHeader(888)
			w.Header().Set("Content-Type", "application/json")
			w.Write(common.Str2byte(_ErrClosingStr))
			return
		}
	}
	ctx := trace.InitTrace(r.Context(), r.Header.Get("Core_tracedata"), this.selfappname, host.Hostip, r.Method, r.URL.Path)
	this.router.ServeHTTP(w, r.Clone(ctx))
	if atomic.LoadInt32(&this.totalreqnum) < 0 {
		select {
		case this.refreshclosewait <- nil:
		default:
		}
	}
	atomic.AddInt32(&this.totalreqnum, -1)
}

func (this *WebServer) insideHandler(method, path string, handlers []OutsideHandler) http.HandlerFunc {
	totalhandlers := append(this.global, handlers...)
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		//cors
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
					w.WriteHeader(http.StatusMethodNotAllowed)
					w.Header().Set("Allow", strings.Join(this.c.Cors.AllowedOrigin, ","))
					w.Header().Set("Content-Type", "application/json")
					w.Write(common.Str2byte(_ErrCorsStr))
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
		traceid, _, _, _, _ := trace.GetTrace(ctx)
		sourceip := r.RemoteAddr
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		if values := r.Header.Values("Core_tracedata"); len(values) != 4 && len(values) != 0 {
			log.Error(ctx, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: tracedata:", values, "format error")
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			w.Write(common.Str2byte(_ErrReqStr))
			return
		} else if len(values) > 0 {
			sourceapp = values[1]
			sourcemethod = values[2]
			sourcepath = values[3]
		}
		var mdata map[string]string
		if mdstr := r.Header.Get("Core_metadata"); mdstr != "" {
			mdata = make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &mdata); e != nil {
				log.Error(ctx, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: metadata:", mdstr, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "application/json")
				w.Write(common.Str2byte(_ErrReqStr))
				return
			}
		}
		var clientdl int64
		if temp := r.Header.Get("Core_deadline"); temp != "" {
			var e error
			clientdl, e = strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error(ctx, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: Deadline:", temp, "format error")
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "application/json")
				w.Write(common.Str2byte(_ErrReqStr))
				return
			}
		}
		//set timeout
		start := time.Now()
		var min int64
		if clientdl != 0 && this.c.GlobalTimeout != 0 {
			if clientdl <= start.UnixNano()+int64(this.c.GlobalTimeout) {
				min = clientdl
			} else {
				min = start.UnixNano() + int64(this.c.GlobalTimeout)
			}
		} else if clientdl != 0 {
			min = clientdl
		} else if this.c.GlobalTimeout != 0 {
			min = start.UnixNano() + int64(this.c.GlobalTimeout)
		}
		if min != 0 {
			if min < start.UnixNano()+int64(time.Millisecond) {
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Header().Set("Content-Type", "application/json")
				w.Write(common.Str2byte(_ErrDeadlineExceededStr))
				end := time.Now()
				trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath), trace.SERVER, this.selfappname, host.Hostip, method, path, &start, &end, cerror.ErrDeadlineExceeded)
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//logic
		workctx := this.getContext(w, r, ctx, sourceapp, mdata, totalhandlers)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[web.server] client:", sourceapp+":"+sourceip, "path:", path, "method:", method, "panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(common.Str2byte(_ErrPanicStr))
				end := time.Now()
				trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath), trace.SERVER, this.selfappname, host.Hostip, method, path, &start, &end, ErrPanic)
			} else {
				end := time.Now()
				trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath), trace.SERVER, this.selfappname, host.Hostip, method, path, &start, &end, workctx.e)
			}
			this.putContext(workctx)
		}()
		workctx.run()
	}
}
