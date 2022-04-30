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
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	//when server close,server will wait at least this time before close
	//min is 1 second
	WaitCloseTime time.Duration
	//mode 0:must have no active requests and must wait at lease WaitCloseTime
	//	every new request come in when the server is closing will refresh the WaitCloseTime
	//mode 1:must have no active requests and must wait at lease WaitCloseTime
	//	WaitCloseTime will not be refreshed by new requests
	WaitCloseMode  int
	ConnectTimeout time.Duration //max time for read the whole request,including the tls handshake
	GlobalTimeout  time.Duration //request's max handling time
	//if this is negative,it is same as disable keep alive,each request will take a new tcp connection,when request finish,tcp closed
	//if this is 0,ConnectTimeout will be used as IdleTimeout
	IdleTimeout time.Duration
	HeartProbe  time.Duration //system's tcp keep alive probe interval,'< 0' disable keep alive,'= 0' will be set to default 15s,min is 1s
	SrcRoot     string        //use /src/relative_path_in_src_root to get the resource in SrcRoot
	MaxHeader   uint
	CertKeys    map[string]string //mapkey: cert path,mapvalue: key path
	Cors        *CorsConfig
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
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
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
	handlerTimeout map[string]map[string]time.Duration //first key method,second key path,value timeout
	handlerRewrite map[string]map[string]string        //first key method,second key origin url,value new url
	selfappname    string
	c              *ServerConfig
	ctxpool        *sync.Pool
	global         []OutsideHandler
	r              *router
	s              *http.Server
	closewait      *sync.WaitGroup
	closewaittimer *time.Timer
	totalreqnum    int32
}

func NewWebServer(c *ServerConfig, selfgroup, selfname string) (*WebServer, error) {
	//pre check
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	//new server
	instance := &WebServer{
		handlerTimeout: make(map[string]map[string]time.Duration),
		handlerRewrite: make(map[string]map[string]string),
		selfappname:    selfappname,
		c:              c,
		ctxpool:        &sync.Pool{},
		global:         make([]OutsideHandler, 0, 10),
		r:              newRouter(c.SrcRoot),
		s: &http.Server{
			ReadTimeout:    c.ConnectTimeout,
			IdleTimeout:    c.IdleTimeout,
			MaxHeaderBytes: int(c.MaxHeader),
			ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
				if c.HeartProbe > 0 {
					(conn.(*net.TCPConn)).SetKeepAlive(true)
					(conn.(*net.TCPConn)).SetKeepAlivePeriod(c.HeartProbe)
				}
				localaddr := conn.LocalAddr().String()
				return context.WithValue(ctx, localport{}, localaddr[strings.LastIndex(localaddr, ":")+1:])
			},
		},
		closewait:      &sync.WaitGroup{},
		closewaittimer: time.NewTimer(0),
	}
	<-instance.closewaittimer.C
	if c.HeartProbe < 0 {
		instance.s.SetKeepAlivesEnabled(false)
	} else {
		instance.s.SetKeepAlivesEnabled(true)
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
	instance.closewait.Add(1)
	instance.r.rewriteHandler = func(originurl, method string) (newurl string, ok bool) {
		handlerRewrite := *(*map[string]map[string]string)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&instance.handlerRewrite))))
		paths, ok := handlerRewrite[method]
		if !ok {
			return
		}
		newurl, ok = paths[originurl]
		return
	}
	instance.r.notFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write(common.Str2byte(cerror.ErrNoapi.Error()))
		log.Error(nil, "[web.server] client ip:", getclientip(r), "call path:", r.URL.Path, "method:", r.Method, "error: unknown path")
	})
	instance.r.optionsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		headers.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
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
	return instance, nil
}

var ErrServerClosed = errors.New("[web.server] closed")

func (s *WebServer) StartWebServer(listenaddr string) error {
	for path := range s.r.getTree.GetAll() {
		log.Info(nil, "[web.server] GET:", path)
	}
	for path := range s.r.postTree.GetAll() {
		log.Info(nil, "[web.server] POST:", path)
	}
	for path := range s.r.putTree.GetAll() {
		log.Info(nil, "[web.server] PUT:", path)
	}
	for path := range s.r.patchTree.GetAll() {
		log.Info(nil, "[web.server] PATCH:", path)
	}
	for path := range s.r.deleteTree.GetAll() {
		log.Info(nil, "[web.server] DELETE:", path)
	}
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	if len(s.c.CertKeys) > 0 {
		//enable h2
		s.s.Handler = s.r
		e = s.s.ServeTLS(l, "", "")
	} else {
		//enable h2c
		s.s.Handler = h2c.NewHandler(s.r, &http2.Server{})
		e = s.s.Serve(l)
	}
	if e != nil {
		if e == http.ErrServerClosed {
			return ErrServerClosed
		}
	}
	return nil
}

//thread unsafe
func (s *WebServer) ReplaceAllPath(newserver *WebServer) {
	for path := range newserver.r.getTree.GetAll() {
		log.Info(nil, "[web.server] GET:", path)
	}
	for path := range newserver.r.postTree.GetAll() {
		log.Info(nil, "[web.server] POST:", path)
	}
	for path := range newserver.r.putTree.GetAll() {
		log.Info(nil, "[web.server] PUT:", path)
	}
	for path := range newserver.r.patchTree.GetAll() {
		log.Info(nil, "[web.server] PATCH:", path)
	}
	for path := range newserver.r.deleteTree.GetAll() {
		log.Info(nil, "[web.server] DELETE:", path)
	}
	if len(s.c.CertKeys) > 0 {
		//enable h2
		s.s.Handler = newserver.r
	} else {
		//enable h2c
		s.s.Handler = h2c.NewHandler(newserver.r, &http2.Server{})
	}
}
func (s *WebServer) StopWebServer() {
	defer s.closewait.Wait()
	for {
		old := atomic.LoadInt32(&s.totalreqnum)
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old-math.MaxInt32) {
				break
			}
		} else {
			return
		}
	}
	//wait at least this.c.WaitCloseTime before stop the under layer socket
	s.closewaittimer.Reset(s.c.WaitCloseTime)
	for {
		<-s.closewaittimer.C
		if atomic.LoadInt32(&s.totalreqnum) != -math.MaxInt32 {
			s.closewaittimer.Reset(s.c.WaitCloseTime)
		} else {
			s.s.Shutdown(context.Background())
			s.closewait.Done()
			return
		}
	}
}

//key origin url,value new url
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
			tmp[method][originurl] = cleanPath(newurl)
		}
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.handlerRewrite)), unsafe.Pointer(&tmp))
}

//first key method,second key path,value timeout(if timeout <= 0 means no timeout)
func (s *WebServer) UpdateHandlerTimeout(htcs map[string]map[string]time.Duration) {
	tmp := make(map[string]map[string]time.Duration)
	for method, paths := range htcs {
		method = strings.ToUpper(method)
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			continue
		}
		for path, timeout := range paths {
			if _, ok := tmp[method]; !ok {
				tmp[method] = make(map[string]time.Duration)
			}
			if len(path) == 0 || path[0] != '/' {
				path = "/" + path
			}
			tmp[method][path] = timeout
		}
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.handlerTimeout)), unsafe.Pointer(&tmp))
}

func (s *WebServer) getHandlerTimeout(method, path string) time.Duration {
	handlerTimeout := *(*map[string]map[string]time.Duration)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.handlerTimeout))))
	if m, ok := handlerTimeout[method]; ok {
		if t, ok := m[path]; ok {
			return t
		}
	}
	return s.c.GlobalTimeout
}

//thread unsafe
func (s *WebServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

//thread unsafe
//path can't start with /src,/src is for static resource
func (s *WebServer) Get(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	if strings.HasPrefix(path, "/src") {
		panic("[web.server] path can't start with /src,/src is for static resource")
	}
	s.r.Get(path, s.insideHandler(http.MethodGet, path, handlers))
}

//thread unsafe
func (s *WebServer) Delete(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Delete(path, s.insideHandler(http.MethodDelete, path, handlers))
}

//thread unsafe
func (s *WebServer) Post(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Post(path, s.insideHandler(http.MethodPost, path, handlers))
}

//thread unsafe
func (s *WebServer) Put(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Put(path, s.insideHandler(http.MethodPut, path, handlers))
}

//thread unsafe
func (s *WebServer) Patch(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Patch(path, s.insideHandler(http.MethodPatch, path, handlers))
}

func (s *WebServer) insideHandler(method, path string, handlers []OutsideHandler) http.HandlerFunc {
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(w http.ResponseWriter, r *http.Request) {
		//target
		if target := r.Header.Get("Core_target"); target != "" && target != s.selfappname {
			//this is not the required server.tell peer self closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(cerror.ErrClosing.Httpcode))
			w.Write(common.Str2byte(cerror.ErrClosing.Error()))
			return
		}
		//cors
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			headers := w.Header()
			headers.Add("Vary", "Origin")
			if s.c.Cors.allorigin {
				headers.Set("Access-Control-Allow-Origin", "*")
			} else {
				find := false
				for _, v := range s.c.Cors.AllowedOrigin {
					if origin == v {
						headers.Set("Access-Control-Allow-Origin", origin)
						find = true
						break
					}
				}
				if !find {
					w.Header().Set("Allow", strings.Join(s.c.Cors.AllowedOrigin, ","))
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
			}
			if s.c.Cors.AllowCredentials {
				headers.Set("Access-Control-Allow-Credentials", "true")
			}
			if len(s.c.Cors.exposestr) > 0 {
				headers.Set("Access-Control-Expose-Headers", s.c.Cors.exposestr)
			}
		}
		//check server status
		for {
			old := atomic.LoadInt32(&s.totalreqnum)
			if old >= 0 {
				//add req num
				if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old+1) {
					break
				}
			} else {
				if s.c.WaitCloseMode == 0 {
					//refresh close wait
					s.closewaittimer.Reset(s.c.WaitCloseTime)
				}
				//tell peer self closed
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(cerror.ErrClosing.Httpcode))
				w.Write(common.Str2byte(cerror.ErrClosing.Error()))
				return
			}
		}
		defer func() {
			if atomic.LoadInt32(&s.totalreqnum) < 0 {
				s.closewaittimer.Reset(s.c.WaitCloseTime)
			}
			atomic.AddInt32(&s.totalreqnum, -1)
		}()
		//trace
		var ctx context.Context
		traceid := ""
		sourceip := getclientip(r)
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		selfdeep := 0
		if tracedata := r.Header.Values("Core_tracedata"); len(tracedata) == 0 || tracedata[0] == "" {
			ctx = trace.InitTrace(r.Context(), "", s.selfappname, host.Hostip, method, path, 0)
		} else if len(tracedata) != 5 || tracedata[4] == "" {
			log.Error(nil, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: tracedata:", tracedata, "format error")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write(common.Str2byte(cerror.ErrReq.Error()))
			return
		} else if clientdeep, e := strconv.Atoi(tracedata[4]); e != nil {
			log.Error(nil, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: tracedata:", tracedata, "format error")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write(common.Str2byte(cerror.ErrReq.Error()))
			return
		} else {
			ctx = trace.InitTrace(r.Context(), tracedata[0], s.selfappname, host.Hostip, method, path, clientdeep)
			sourceapp = tracedata[1]
			sourcemethod = tracedata[2]
			sourcepath = tracedata[3]
		}
		traceid, _, _, _, _, selfdeep = trace.GetTrace(ctx)
		var mdata map[string]string
		if mdstr := r.Header.Get("Core_metadata"); mdstr != "" {
			mdata = make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &mdata); e != nil {
				log.Error(ctx, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: metadata:", mdstr, "format error")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(cerror.ErrReq.Error()))
				return
			}
		}
		var clientdl int64
		if temp := r.Header.Get("Core_deadline"); temp != "" {
			var e error
			clientdl, e = strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error(ctx, "[web.server] client ip:", getclientip(r), "path:", path, "method:", method, "error: Deadline:", temp, "format error")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(cerror.ErrReq.Error()))
				return
			}
		}
		//set timeout
		start := time.Now()
		var min int64
		servertimeout := int64(s.getHandlerTimeout(method, path))
		if servertimeout > 0 {
			serverdl := start.UnixNano() + servertimeout
			if clientdl != 0 {
				//compare use the small one
				if clientdl < serverdl {
					min = clientdl
				} else {
					min = serverdl
				}
			} else {
				//use serverdl
				min = serverdl
			}
		} else if clientdl != 0 {
			//use client timeout
			min = clientdl
		} else {
			//no timeout
			min = 0
		}
		if min != 0 {
			if min < start.UnixNano()+int64(time.Millisecond) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Write(common.Str2byte(cerror.ErrDeadlineExceeded.Error()))
				end := time.Now()

				trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), trace.SERVER, s.selfappname, host.Hostip+":"+r.Context().Value(localport{}).(string), method, path, &start, &end, cerror.ErrDeadlineExceeded)
				monitor.WebServerMonitor(sourceapp, method, path, cerror.ErrDeadlineExceeded, uint64(end.UnixNano()-start.UnixNano()))
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//logic
		workctx := s.getContext(w, r, ctx, sourceapp, mdata, totalhandlers)
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[web.server] client:", sourceapp+":"+sourceip, "path:", path, "method:", method, "panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(common.Str2byte(cerror.ErrPanic.Error()))
				workctx.e = cerror.ErrPanic
			}
			end := time.Now()
			trace.Trace(trace.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), trace.SERVER, s.selfappname, host.Hostip+":"+r.Context().Value(localport{}).(string), method, path, &start, &end, workctx.e)
			monitor.WebServerMonitor(sourceapp, method, path, workctx.e, uint64(end.UnixNano()-start.UnixNano()))
			s.putContext(workctx)
		}()
		workctx.run()
	}
}

type localport struct{}
