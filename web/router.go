package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/container/trie"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
)

type Router struct {
	s          *WebServer
	globalmids []OutsideHandler
	getTree    *trie.Trie[http.HandlerFunc]
	postTree   *trie.Trie[http.HandlerFunc]
	putTree    *trie.Trie[http.HandlerFunc]
	patchTree  *trie.Trie[http.HandlerFunc]
	deleteTree *trie.Trie[http.HandlerFunc]
	srcroot    fs.FS
}

// the first character must be slash(/)
//
//	api/abc -> /api/abc
//
// remove tail slash(/)
//
//	/api/abc/ -> /api/abc
//
// multi series slash(///) -> single slash(/)
//
//	/api//abc -> /api/abc
//
// . -> current dir
//
//	/api/abc/. -> /api/abc                   (match)
//	/api/./abc -> /api/abc                   (match)
//	/api./abc  -> /api./abc                  (not match)
//	/api/.abc  -> /api/.abc                  (not match)
//
// .. -> parent dir
//
//	/api/abc/xyz/.. -> /api/abc              (match)
//	/api/abc/../xyz -> /api/xyz              (match)
//	/api/abc/.../xyz -> /api/abc/.../xyz     (not match)
//	/api/abc../xyz -> /api/abc../xyz         (not match)
//	/api/abc/..xyz -> /api/abc/..xyz         (not match)
func cleanPath(origin string) string {
	if origin == "" {
		return "/"
	}
	var realpos int
	buf := pool.GetPool().Get(len(origin) + 1) // +1 for not start from '/'
	defer pool.GetPool().Put(&buf)
	if origin[0] != '/' {
		buf[0] = '/'
		realpos = 1
	}
	for i, v := range common.STB(origin) {
		if v == '/' {
			if realpos == 0 || buf[realpos-1] != '/' {
				buf[realpos] = v
				realpos++
			}
			continue
		}
		if v == '.' {
			if buf[realpos-1] != '/' {
				buf[realpos] = v
				realpos++
				continue
			}
			if i == len(origin)-1 {
				if realpos > 1 {
					realpos--
				}
				break
			}
			if origin[i+1] == '/' {
				continue
			}
			if origin[i+1] == '.' {
				if i+1 == len(origin)-1 {
					if realpos > 1 {
						realpos--
						for {
							if buf[realpos-1] == '/' {
								break
							}
							realpos--
						}
					}
					break
				}
				if origin[i+2] == '/' {
					if realpos > 1 {
						realpos--
						for {
							if buf[realpos-1] == '/' {
								break
							}
							realpos--
						}
					}
					continue
				}
			}
		}
		buf[realpos] = v
		realpos++
	}
	if realpos > 1 && buf[realpos-1] == '/' {
		realpos--
	}
	return string(buf[:realpos])
}
func realip(r *http.Request) string {
	ip := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
		if ip != "" {
			return ip
		}
	}
	if ip = strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip == "" {
		ip, _, _ = net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	}
	return ip
}

// thread unsafe
func (r *Router) Use(globalMids ...OutsideHandler) {
	r.globalmids = append(r.globalmids, globalMids...)
}

// thread unsafe
func (r *Router) Get(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.getTree.Set(path, r.insideHandler("GET", path, handlers))
}

// thread unsafe
func (r *Router) Post(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.postTree.Set(path, r.insideHandler("POST", path, handlers))
}

// thread unsafe
func (r *Router) Patch(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.patchTree.Set(path, r.insideHandler("PATCH", path, handlers))
}

// thread unsafe
func (r *Router) Put(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.putTree.Set(path, r.insideHandler("PUT", path, handlers))
}

// thread unsafe
func (r *Router) Delete(path string, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.putTree.Set(path, r.insideHandler("DELETE", path, handlers))
}
func (r *Router) insideHandler(method, path string, handlers []OutsideHandler) http.HandlerFunc {
	totalhandlers := make([]OutsideHandler, len(r.globalmids)+len(handlers))
	copy(totalhandlers, r.globalmids)
	copy(totalhandlers[len(r.globalmids):], handlers)
	return func(resp http.ResponseWriter, req *http.Request) {
		//target
		if target := req.Header.Get("Core-Target"); target != "" && target != r.s.self {
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrTarget.Httpcode))
			resp.Write(common.STB(cerror.ErrTarget.Error()))
			return
		}
		//trace
		var ctx context.Context
		var span *trace.Span
		peerip := realip(req)
		if traceparentstr := req.Header.Get("Traceparent"); traceparentstr != "" {
			tid, psid, e := trace.ParseTraceParent(traceparentstr)
			if e != nil {
				log.Error(nil, "[web.server] trace data format wrong",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("method", method),
					log.String("trace_parent", traceparentstr))
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrReq.Httpcode))
				resp.Write(common.STB(cerror.ErrReq.Error()))
				return
			}
			parent := trace.NewSpanData(tid, psid)
			if tracestatestr := req.Header.Get("Tracestate"); tracestatestr != "" {
				tmp, e := trace.ParseTraceState(tracestatestr)
				if e != nil {
					log.Error(nil, "[web.server] trace data format wrong",
						log.String("cip", peerip),
						log.String("path", path),
						log.String("method", method),
						log.String("trace_state", tracestatestr))
					resp.Header().Set("Content-Type", "application/json")
					resp.WriteHeader(int(cerror.ErrReq.Httpcode))
					resp.Write(common.STB(cerror.ErrReq.Error()))
					return
				}
				var app, host, method, path bool
				for k, v := range tmp {
					switch k {
					case "app":
						app = true
					case "host":
						host = true
						peerip = v
					case "method":
						method = true
					case "path":
						path = true
					}
					parent.SetStateKV(k, v)
				}
				if !app {
					parent.SetStateKV("app", "unknown")
				}
				if !host {
					parent.SetStateKV("host", peerip)
				}
				if !method {
					parent.SetStateKV("method", "unknown")
				}
				if !path {
					parent.SetStateKV("path", "unknown")
				}
			}
			ctx, span = trace.NewSpan(req.Context(), "Corelib.Web", trace.Server, parent)
		} else {
			ctx, span = trace.NewSpan(req.Context(), "Corelib.Web", trace.Server, nil)
			span.GetParentSpanData().SetStateKV("app", "unknown")
			span.GetParentSpanData().SetStateKV("host", peerip)
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		}
		span.GetSelfSpanData().SetStateKV("app", r.s.self)
		span.GetSelfSpanData().SetStateKV("host", host.Hostip)
		span.GetSelfSpanData().SetStateKV("method", method)
		span.GetSelfSpanData().SetStateKV("path", path)
		var md map[string]string
		if mdstr := req.Header.Get("Core-Metadata"); mdstr != "" {
			md = make(map[string]string)
			if e := json.Unmarshal(common.STB(mdstr), &md); e != nil {
				log.Error(ctx, "[web.server] meta data format wrong",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("method", method),
					log.String("metadata", mdstr))
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrReq.Httpcode))
				resp.Write(common.STB(cerror.ErrReq.Error()))
				span.Finish(cerror.ErrReq)
				return
			}
		}
		if md == nil {
			md = map[string]string{"Client-IP": peerip}
		} else if _, ok := md["Client-IP"]; !ok {
			md["Client-IP"] = peerip
		}
		//client timeout
		if temp := req.Header.Get("Core-Deadline"); temp != "" {
			clientdl, e := strconv.ParseInt(temp, 10, 64)
			if e != nil {
				log.Error(ctx, "[web.server] deadline format wrong",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("method", method),
					log.String("deadline", temp))
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrReq.Httpcode))
				resp.Write(common.STB(cerror.ErrReq.Error()))
				span.Finish(cerror.ErrReq)
				return
			}
			if clientdl != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Unix(0, clientdl))
				defer cancel()
			}
		}
		//check server status
		if e := r.s.stop.Add(1); e != nil {
			if r.s.c.WaitCloseMode == 0 {
				//refresh close wait
				r.s.closetimer.Reset(r.s.c.WaitCloseTime.StdDuration())
			}
			if e == graceful.ErrClosing {
				//tell peer self closed
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrServerClosing.Httpcode))
				resp.Write(common.STB(cerror.ErrServerClosing.Error()))
				span.Finish(cerror.ErrServerClosing)
			} else {
				//tell peer self busy
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrBusy.Httpcode))
				resp.Write(common.STB(cerror.ErrBusy.Error()))
				span.Finish(cerror.ErrBusy)
			}
			return
		}
		defer r.s.stop.DoneOne()
		//logic
		workctx := r.s.getContext(metadata.SetMetadata(ctx, md), resp, req, peerip, totalhandlers)
		paniced := true
		defer func() {
			if paniced {
				e := recover()
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[web.server] panic",
					log.String("cip", peerip),
					log.String("path", path),
					log.String("method", method),
					log.Any("panic", e),
					log.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				resp.Header().Set("Cpu-Usage", strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64))
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(http.StatusInternalServerError)
				resp.Write(common.STB(cerror.ErrPanic.Error()))
				workctx.e = cerror.ErrPanic
			}
			span.Finish(workctx.e)
			peername, _ := span.GetParentSpanData().GetStateKV("app")
			monitor.WebServerMonitor(peername, method, path, workctx.e, uint64(span.GetEnd()-span.GetStart()))
			r.s.putContext(workctx)
		}()
		workctx.run()
		paniced = false
	}
}
func (r *Router) notFoundHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write(common.STB(cerror.ErrNotExist.Error()))
	log.Error(nil, "[web.server] path not exist",
		log.String("cip", realip(req)),
		log.String("path", req.URL.Path),
		log.String("method", req.Method))
}
func (r *Router) srcFileHandler(resp http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	if file, e := r.srcroot.Open(path[1:]); e != nil {
		if os.IsNotExist(e) {
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrNotExist.Httpcode))
			resp.Write(common.STB(cerror.ErrNotExist.Error()))
			log.Error(nil, "[web.server] static src file not exist",
				log.String("cip", realip(req)),
				log.String("path", req.URL.Path),
				log.String("method", req.Method))
		} else {
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrSystem.Httpcode))
			resp.Write(common.STB(cerror.ErrSystem.Error()))
			log.Error(nil, "[web.server] open static src file failed",
				log.String("cip", realip(req)),
				log.String("path", req.URL.Path),
				log.String("method", req.Method),
				log.CError(e))
		}
	} else if fileinfo, e := file.Stat(); e != nil {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrSystem.Httpcode))
		resp.Write(common.STB(cerror.ErrSystem.Error()))
		log.Error(nil, "[web.server] get static src file info failed",
			log.String("cip", realip(req)),
			log.String("path", req.URL.Path),
			log.String("method", req.Method),
			log.CError(e))
		file.Close()
	} else if !fileinfo.Mode().IsRegular() {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrNotExist.Httpcode))
		resp.Write(common.STB(cerror.ErrNotExist.Error()))
		log.Error(nil, "[web.server] static src file not exist",
			log.String("cip", realip(req)),
			log.String("path", req.URL.Path),
			log.String("method", req.Method))
		file.Close()
	} else {
		http.ServeContent(resp, req, fileinfo.Name(), fileinfo.ModTime(), file.(*os.File))
		file.Close()
	}
}
func (r *Router) corsOptions(resp http.ResponseWriter, req *http.Request) {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		resp.WriteHeader(http.StatusNoContent)
		return
	}
	resp.Header().Add("Vary", "Origin")
	for _, v := range r.s.c.CorsAllowedOrigins {
		if v == "*" {
			resp.Header().Set("Access-Control-Allow-Origin", "*")
			break
		} else if v == origin {
			resp.Header().Set("Access-Control-Allow-Origin", origin)
			break
		} else if strings.Contains(v, "*") {
			pieces := strings.Split(v, "*")
			index := 0
			for _, piece := range pieces {
				if len(piece) == 0 {
					continue
				}
				i := strings.Index(origin[index:], piece)
				if i == -1 {
					break
				}
				index += i + len(piece)
			}
			if index == len(origin) {
				resp.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
	}
	if resp.Header().Get("Access-Control-Allow-Origin") == "" {
		resp.WriteHeader(http.StatusForbidden)
		log.Error(nil, "[web.server] cors check failed",
			log.String("cip", realip(req)),
			log.String("path", req.URL.Path),
			log.String("method", req.Method))
		return
	}
	if r.s.c.CorsAllowCredentials {
		resp.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	resp.Header().Add("Vary", "Access-Control-Request-Method")
	resp.Header().Add("Vary", "Access-Control-Request-Headers")
	resp.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
	if len(r.s.c.CorsAllowedHeaders) == 1 && r.s.c.CorsAllowedHeaders[0] == "*" {
		resp.Header().Set("Access-Control-Allow-Headers", "*")
	} else if len(r.s.c.CorsAllowedHeaders) > 0 {
		resp.Header().Set("Access-Control-Allow-Headers", strings.Join(r.s.c.CorsAllowedHeaders, ","))
	}
	if r.s.c.CorsMaxAge > 0 {
		resp.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(r.s.c.CorsMaxAge.StdDuration().Seconds())))
	}
	resp.WriteHeader(http.StatusNoContent)
	return
}
func (r *Router) corsNormal(resp http.ResponseWriter, req *http.Request) bool {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	resp.Header().Add("Vary", "Origin")
	for _, v := range r.s.c.CorsAllowedOrigins {
		if v == "*" {
			resp.Header().Set("Access-Control-Allow-Origin", "*")
			break
		} else if v == origin {
			resp.Header().Set("Access-Control-Allow-Origin", origin)
			break
		} else if strings.Contains(v, "*") {
			pieces := strings.Split(v, "*")
			index := 0
			for _, piece := range pieces {
				if len(piece) == 0 {
					continue
				}
				i := strings.Index(origin[index:], piece)
				if i == -1 {
					break
				}
				index += i + len(piece)
			}
			if index == len(origin) {
				resp.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
	}
	if resp.Header().Get("Access-Control-Allow-Origin") == "" {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrCors.Httpcode))
		resp.Write(common.STB(cerror.ErrCors.Error()))
		log.Error(nil, "[web.server] cors check failed",
			log.String("cip", realip(req)),
			log.String("path", req.URL.Path),
			log.String("method", req.Method))
		return false
	}
	if r.s.c.CorsAllowCredentials {
		resp.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if len(r.s.c.CorsExposeHeaders) == 1 && r.s.c.CorsExposeHeaders[0] == "*" {
		resp.Header().Set("Access-Control-Expose-Headers", "*")
	} else if len(r.s.c.CorsExposeHeaders) > 0 {
		resp.Header().Set("Access-Control-Expose-Headers", strings.Join(r.s.c.CorsExposeHeaders, ","))
	}
	return true
}

func (r *Router) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		realmethod := strings.ToUpper(req.Header.Get("Access-Control-Request-Method"))
		if url, ok := r.s.getHandlerRewrite(req.URL.Path, realmethod); ok {
			req.URL.Path = url
		}
		r.corsOptions(resp, req)
		return
	}
	if url, ok := r.s.getHandlerRewrite(req.URL.Path, req.Method); ok {
		req.URL.Path = url
	}
	if !r.corsNormal(resp, req) {
		return
	}
	var handler http.HandlerFunc
	var ok bool
	switch req.Method {
	case http.MethodGet:
		handler, ok = r.getTree.Get(req.URL.Path)
	case http.MethodPost:
		handler, ok = r.postTree.Get(req.URL.Path)
	case http.MethodPut:
		handler, ok = r.putTree.Get(req.URL.Path)
	case http.MethodPatch:
		handler, ok = r.patchTree.Get(req.URL.Path)
	case http.MethodDelete:
		handler, ok = r.deleteTree.Get(req.URL.Path)
	}
	if !ok {
		req.URL.Path = cleanPath(req.URL.Path)
		switch req.Method {
		case http.MethodGet:
			handler, _ = r.getTree.Get(req.URL.Path)
		case http.MethodPost:
			handler, _ = r.postTree.Get(req.URL.Path)
		case http.MethodPut:
			handler, _ = r.putTree.Get(req.URL.Path)
		case http.MethodPatch:
			handler, _ = r.patchTree.Get(req.URL.Path)
		case http.MethodDelete:
			handler, _ = r.deleteTree.Get(req.URL.Path)
		}
	}
	if handler == nil && (r.srcroot == nil || req.Method != http.MethodGet) {
		r.notFoundHandler(resp, req)
		return
	} else if handler == nil {
		//handler static source file
		handler = r.srcFileHandler
	}
	timeout := r.s.getHandlerTimeout(req.URL.Path, req.Method)
	if timeout > 0 {
		http.TimeoutHandler(handler, timeout, cerror.ErrDeadlineExceeded.Error()).ServeHTTP(resp, req)
	} else {
		handler(resp, req)
	}
}
func (r *Router) printPath() {
	rewrite := r.s.handlerRewrite
	for path := range r.getTree.GetAll() {
		log.Info(nil, "[web.server] GET: "+path)
	}
	if rewrite, ok := rewrite["GET"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] GET: "+ourl+" => "+nurl)
		}
	}
	for path := range r.postTree.GetAll() {
		log.Info(nil, "[web.server] POST: "+path)
	}
	if rewrite, ok := rewrite["POST"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] POST: "+ourl+" => "+nurl)
		}
	}
	for path := range r.putTree.GetAll() {
		log.Info(nil, "[web.server] PUT: "+path)
	}
	if rewrite, ok := rewrite["PUT"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] PUT: "+ourl+" => "+nurl)
		}
	}
	for path := range r.patchTree.GetAll() {
		log.Info(nil, "[web.server] PATCH: "+path)
	}
	if rewrite, ok := rewrite["PATCH"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] PATCH: "+ourl+" => "+nurl)
		}
	}
	for path := range r.deleteTree.GetAll() {
		log.Info(nil, "[web.server] DELETE: "+path)
	}
	if rewrite, ok := rewrite["DELETE"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] DELETE: "+ourl+" => "+nurl)
		}
	}
}
