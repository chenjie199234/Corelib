package web

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
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
	SrcRoot     string
	MaxHeader   uint
	Certs       map[string]string //mapkey: cert path,mapvalue: private key path
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
	selfapp    string
	c          *ServerConfig
	ctxpool    *sync.Pool
	global     []OutsideHandler
	r          *router
	s          *http.Server
	clientnum  int32
	stop       *graceful.Graceful
	closetimer *time.Timer
}

func NewWebServer(c *ServerConfig, selfappgroup, selfappname string) (*WebServer, error) {
	//pre check
	selfapp := selfappgroup + "." + selfappname
	if e := name.FullCheck(selfapp); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	//new server
	instance := &WebServer{
		selfapp:    selfapp,
		c:          c,
		ctxpool:    &sync.Pool{},
		global:     make([]OutsideHandler, 0, 10),
		r:          newRouter(c.SrcRoot),
		stop:       graceful.New(),
		closetimer: time.NewTimer(0),
	}
	<-instance.closetimer.C
	instance.s = &http.Server{
		ReadTimeout:    c.ConnectTimeout,
		IdleTimeout:    c.IdleTimeout,
		MaxHeaderBytes: int(c.MaxHeader),
		ConnState: func(c net.Conn, s http.ConnState) {
			if s == http.StateNew {
				atomic.AddInt32(&instance.clientnum, 1)
			} else if s == http.StateHijacked || s == http.StateClosed {
				atomic.AddInt32(&instance.clientnum, -1)
			}
		},
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			if c.HeartProbe > 0 {
				if len(c.Certs) > 0 {
					((conn.(*tls.Conn)).NetConn().(*net.TCPConn)).SetKeepAlive(true)
					((conn.(*tls.Conn)).NetConn().(*net.TCPConn)).SetKeepAlivePeriod(c.HeartProbe)
				} else {
					(conn.(*net.TCPConn)).SetKeepAlive(true)
					(conn.(*net.TCPConn)).SetKeepAlivePeriod(c.HeartProbe)
				}
			}
			localaddr := conn.LocalAddr().String()
			return context.WithValue(ctx, localport{}, localaddr[strings.LastIndex(localaddr, ":")+1:])
		},
	}
	if c.HeartProbe < 0 {
		instance.s.SetKeepAlivesEnabled(false)
	} else {
		instance.s.SetKeepAlivesEnabled(true)
	}
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[web.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		instance.s.TLSConfig = &tls.Config{Certificates: certificates}
	}
	instance.r.notFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write(common.Str2byte(cerror.ErrNotExist.Error()))
		log.Error(nil, "[web.server] path not exist", map[string]interface{}{"cip": realip(r), "path": r.URL.Path, "method": r.Method})
	})
	instance.r.srcPermissionHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write(common.Str2byte(cerror.ErrPermission.Error()))
		log.Error(nil, "[web.server] static src file permission denie", map[string]interface{}{"cip": realip(r), "path": r.URL.Path, "method": r.Method})
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
	laddr, e := net.ResolveTCPAddr("tcp", listenaddr)
	if e != nil {
		return errors.New("[web.server] resolve addr:" + listenaddr + " error:" + e.Error())
	}
	l, e := net.ListenTCP("tcp", laddr)
	if e != nil {
		return errors.New("[web.server] listen addr:" + listenaddr + " error:" + e.Error())
	}
	s.r.printPath()
	if len(s.c.Certs) > 0 {
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
func (s *WebServer) GetClientNum() int32 {
	return s.clientnum
}
func (s *WebServer) GetReqNum() int64 {
	return s.stop.GetNum()
}

// thread unsafe
func (s *WebServer) ReplaceAllPath(newserver *WebServer) {
	s.r = newserver.r
	s.r.printPath()
	if len(s.c.Certs) > 0 {
		//enable h2
		s.s.Handler = s.r
	} else {
		//enable h2c
		s.s.Handler = h2c.NewHandler(s.r, &http2.Server{})
	}
}
func (s *WebServer) StopWebServer(force bool) {
	if force {
		s.s.Close()
	} else {
		s.stop.Close(nil, nil)
		//wait at least this.c.WaitCloseTime before stop the under layer socket
		s.closetimer.Reset(s.c.WaitCloseTime)
		<-s.closetimer.C
		s.s.Shutdown(context.Background())
	}
}
func (s *WebServer) UpdateSrcRoot(srcroot string) {
	s.r.UpdateSrcRoot(srcroot)
}

// key origin url,value new url
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
	s.r.updaterewrite(tmp)
}

// first key method,second key path,value timeout(if timeout <= 0 means no timeout)
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
	tmp["default"] = map[string]time.Duration{"default": s.c.GlobalTimeout}
	s.r.updatetimeout(tmp)
}

// thread unsafe
func (s *WebServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *WebServer) Get(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Get(path, s.insideHandler(http.MethodGet, path, handlers))
}

// thread unsafe
func (s *WebServer) Delete(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Delete(path, s.insideHandler(http.MethodDelete, path, handlers))
}

// thread unsafe
func (s *WebServer) Post(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Post(path, s.insideHandler(http.MethodPost, path, handlers))
}

// thread unsafe
func (s *WebServer) Put(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	s.r.Put(path, s.insideHandler(http.MethodPut, path, handlers))
}

// thread unsafe
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
		if target := r.Header.Get("Core-Target"); target != "" && target != s.selfapp {
			//this is not the required server.tell peer self closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(cerror.ErrTarget.Httpcode))
			w.Write(common.Str2byte(cerror.ErrTarget.Error()))
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
		//trace
		var ctx context.Context
		sourceip := realip(r)
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		if tracestr := r.Header.Get("Core-Tracedata"); tracestr != "" {
			tracedata := make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(tracestr), &tracedata); e != nil {
				log.Error(nil, "[web.server] tracedata format wrong", map[string]interface{}{"cip": sourceip, "path": path, "method": method, "tracedata": tracestr})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(cerror.ErrReq.Error()))
				return
			}
			if len(tracedata) == 0 || tracedata["TraceID"] == "" {
				ctx = log.InitTrace(r.Context(), "", s.selfapp, host.Hostip, method, path, 0)
			} else {
				sourceapp = tracedata["SourceApp"]
				sourcemethod = tracedata["SourceMethod"]
				sourcepath = tracedata["SourcePath"]
				clientdeep, e := strconv.Atoi(tracedata["Deep"])
				if e != nil || sourceapp == "" || sourcemethod == "" || sourcepath == "" || clientdeep == 0 {
					log.Error(nil, "[web.server] tracedata format wrong", map[string]interface{}{"cip": sourceip, "path": path, "method": method, "tracedata": tracestr})
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					w.Write(common.Str2byte(cerror.ErrReq.Error()))
					return
				}
				ctx = log.InitTrace(r.Context(), tracedata["TraceID"], s.selfapp, host.Hostip, method, path, clientdeep)
			}
		} else {
			ctx = log.InitTrace(r.Context(), "", s.selfapp, host.Hostip, method, path, 0)
		}
		traceid, _, _, _, _, selfdeep := log.GetTrace(ctx)
		var mdata map[string]string
		if mdstr := r.Header.Get("Core-Metadata"); mdstr != "" {
			mdata = make(map[string]string)
			if e := json.Unmarshal(common.Str2byte(mdstr), &mdata); e != nil {
				log.Error(ctx, "[web.server] metadata format wrong", map[string]interface{}{"cname": sourceapp, "cip": sourceip, "path": path, "method": method, "metadata": mdstr})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(cerror.ErrReq.Error()))
				return
			}
		}
		//timeout
		if temp := r.Header.Get("Core-Deadline"); temp != "" {
			clientdl, e := strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error(ctx, "[web.server] deadline format wrong", map[string]interface{}{"cname": sourceapp, "cip": sourceip, "path": path, "method": method, "deadline": temp})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write(common.Str2byte(cerror.ErrReq.Error()))
				return
			}
			if clientdl != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Unix(0, clientdl))
				defer cancel()
			}
		}
		//check server status
		if !s.stop.AddOne() {
			if s.c.WaitCloseMode == 0 {
				//refresh close wait
				s.closetimer.Reset(s.c.WaitCloseTime)
			}
			//tell peer self closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(cerror.ErrServerClosing.Httpcode))
			w.Write(common.Str2byte(cerror.ErrServerClosing.Error()))
			return
		}
		defer s.stop.DoneOne()
		//logic
		start := time.Now()
		workctx := s.getContext(w, r, ctx, sourceapp, mdata, totalhandlers)
		if _, ok := workctx.metadata["Client-IP"]; !ok {
			workctx.metadata["Client-IP"] = sourceip
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[web.server] panic", map[string]interface{}{"cname": sourceapp, "cip": sourceip, "path": path, "method": method, "panic": e, "stack": base64.StdEncoding.EncodeToString(stack[:n])})
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(common.Str2byte(cerror.ErrPanic.Error()))
				workctx.e = cerror.ErrPanic
			}
			end := time.Now()
			log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfapp, host.Hostip+":"+r.Context().Value(localport{}).(string), method, path, &start, &end, workctx.e)
			monitor.WebServerMonitor(sourceapp, method, path, workctx.e, uint64(end.UnixNano()-start.UnixNano()))
			s.putContext(workctx)
		}()
		workctx.run()
	}
}

type localport struct{}
