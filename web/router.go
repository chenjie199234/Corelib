package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/cotel"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type handler struct {
	r             *Router
	method        string
	path          string
	sse           bool
	totalhandlers []OutsideHandler
	timeout       atomic.Int64 /*time.Duration*/
}

func (h *handler) handle(resp http.ResponseWriter, req *http.Request) {
	//target
	if target := req.Header.Get("Core-Target"); target != "" && target != name.GetSelfFullName() {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrTarget.GetHttpcode()))
		resp.Write(common.STB(cerror.ErrTarget.Json()))
		return
	}
	//check server status
	if e := h.r.s.stop.Add(1); e != nil {
		if h.r.s.c.WaitCloseMode == 0 {
			//refresh close wait
			h.r.s.closetimer.Reset(h.r.s.c.WaitCloseTime.StdDuration())
		}
		if e == graceful.ErrClosing {
			//tell peer self closed
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrServerClosing.GetHttpcode()))
			resp.Write(common.STB(cerror.ErrServerClosing.Json()))
		} else {
			//tell peer self busy
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrBusy.GetHttpcode()))
			resp.Write(common.STB(cerror.ErrBusy.Json()))
		}
		return
	}

	peerip := realip(req)

	//trace
	clientname := req.Header.Get("Core-Self")
	if clientname == "" {
		clientname = "unknown"
	}
	ctx, span := h.r.s.tracer.Start(
		otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header)),
		"handle web",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("path", h.path), attribute.String("cname", clientname), attribute.String("cip", peerip)))
	var stime int64 //only used in metric
	if cotel.NeedMetric() {
		if cotel.NeedTrace() {
			stime = span.(sdktrace.ReadOnlySpan).StartTime().UnixNano()
		} else {
			stime = time.Now().UnixNano()
		}
	}
	//metadata
	var md map[string]string
	if mdstr := req.Header.Get("Core-Metadata"); mdstr != "" {
		md = make(map[string]string)
		if e := json.Unmarshal(common.STB(mdstr), &md); e != nil {
			slog.ErrorContext(ctx, "[web.server] metadata format wrong",
				slog.String("cip", peerip),
				slog.String("path", h.path),
				slog.String("method", h.method),
				slog.String("metadata", mdstr))
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrReq.GetHttpcode()))
			resp.Write(common.STB(cerror.ErrReq.Json()))
			span.SetStatus(codes.Error, cerror.ErrReq.Error())
			span.End()
			h.r.s.stop.DoneOne()
			return
		}
	}
	if md == nil {
		md = map[string]string{"Client-IP": peerip}
	} else if _, ok := md["Client-IP"]; !ok {
		md["Client-IP"] = peerip
	}

	//deadline
	var cdl int64
	if temp := req.Header.Get("Core-Deadline"); temp != "" {
		var e error
		cdl, e = strconv.ParseInt(temp, 10, 64)
		if e != nil {
			slog.Error("[web.server] deadline format wrong",
				slog.String("cip", peerip),
				slog.String("path", h.path),
				slog.String("method", h.method),
				slog.String("deadline", temp))
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrReq.GetHttpcode()))
			resp.Write(common.STB(cerror.ErrReq.Json()))
			span.SetStatus(codes.Error, cerror.ErrReq.Error())
			span.End()
			h.r.s.stop.DoneOne()
			return
		}
	}
	var dl time.Time
	if sto := h.timeout.Load(); sto > 0 {
		//use server deadline
		dl = time.Now().Add(time.Duration(sto))
		if cdl > 0 && cdl < dl.UnixNano() {
			//use client deadline
			dl = time.Unix(0, cdl)
		}
	} else if cdl > 0 {
		//use client deadline
		dl = time.Unix(0, cdl)
	}
	if !dl.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
		//reset the write deadline on the raw socket
		respc := http.NewResponseController(resp)
		respc.SetWriteDeadline(dl)
	} else {
		//reset the write deadline on the raw socket
		respc := http.NewResponseController(resp)
		respc.SetWriteDeadline(time.Time{})
	}

	//logic
	wctx := &ServerContext{
		Context: metadata.SetMetadata(ctx, md),
		sse:     h.sse,
		w:       resp,
		r:       req,
		peerip:  peerip,
	}
	internalhandler := func() {
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				slog.ErrorContext(wctx, "[web.server] panic",
					slog.String("cip", peerip),
					slog.String("path", h.path),
					slog.String("method", h.method),
					slog.Any("panic", e),
					slog.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				wctx.Abort(cerror.ErrPanic)
			}
			if wctx.e != nil {
				span.SetStatus(codes.Error, wctx.e.Error())
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
			if cotel.NeedMetric() {
				var etime int64
				if cotel.NeedTrace() {
					etime = span.(sdktrace.ReadOnlySpan).EndTime().UnixNano()
				} else {
					etime = time.Now().UnixNano()
				}
				attr := attribute.String("path", h.path)
				if wctx.e != nil {
					h.r.s.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "error")))
				} else {
					h.r.s.statusCounter.Add(context.Background(), 1, metric.WithAttributes(attr, attribute.String("status", "ok")))
				}
				h.r.s.timeHistogram.Record(context.Background(), float64(etime-stime)/1000000.0, metric.WithAttributes(attr))
			}
			h.r.s.stop.DoneOne()
		}()
		for _, handler := range h.totalhandlers {
			handler(wctx)
			if wctx.closed.Load() {
				break
			}
		}
		if !wctx.closed.Load() {
			wctx.Abort(nil)
		}
	}
	if dl.IsZero() {
		//run in sync mode
		internalhandler()
	} else {
		//run in async mode
		wctx.lker = &sync.Mutex{}
		done := make(chan struct{})
		go func() {
			internalhandler()
			close(done)
		}()
		select {
		case <-ctx.Done():
			select {
			case <-done:
				//ctx and done will compete if they fired at the same time
				//but the done should have the high priority
			default:
				switch ctx.Err() {
				case context.DeadlineExceeded:
					//need to check the early return
					wctx.lker.Lock()
					if !wctx.closed.Swap(true) {
						wctx.lker.Unlock()
						//not aborted
						if !wctx.responsed.Swap(true) {
							//not responsed
							resp.Header().Set("Content-Type", "application/json")
							resp.WriteHeader(int(cerror.ErrDeadlineExceeded.GetHttpcode()))
							resp.Write(common.STB(cerror.ErrDeadlineExceeded.Json()))
						} else if wctx.sse {
							//already responsed in ServerSentEvent mode
							d := cerror.ErrDeadlineExceeded.Json()
							msg := make([]byte, 0, 21+len(d))
							msg = append(msg, "event: error\ndata: "...)
							msg = append(msg, d...)
							msg = append(msg, "\n\n"...)
							resp.Write(msg)
						} else {
							//already responsed in normal mode
							//we don't known how to handle this
						}
					} else {
						wctx.lker.Unlock()
					}
				case context.Canceled:
					//only when the client leave,the connection close will enter this
					//the client already gone,we don't need to return early,wait the handler return here
					<-done
				}
			}
		case <-done:
		}
	}
}

type Router struct {
	s              *WebServer
	status         bool //true - working
	lker           sync.Mutex
	handlertimeout map[string]map[string]time.Duration //first key method,second key path,value timeout,<=0 means no timeout
	handlerrewrite map[string]map[string]string        //first key method,second key origin url,value new url
	globalmids     []OutsideHandler
	tmpget         map[string]*handler
	tmppost        map[string]*handler
	tmpput         map[string]*handler
	tmppatch       map[string]*handler
	tmpdelete      map[string]*handler
	get            atomic.Pointer[map[string]*handler]
	post           atomic.Pointer[map[string]*handler]
	put            atomic.Pointer[map[string]*handler]
	patch          atomic.Pointer[map[string]*handler]
	delete         atomic.Pointer[map[string]*handler]
	srcroot        fs.FS
}

// first key method,second key origin url,value new url
// empty origin url or new url will be ignored
func (r *Router) UpdateHandlerRewrite(rewrite map[string]map[string]string) {
	r.lker.Lock()
	defer r.lker.Unlock()
	tmp := make(map[string]map[string]string)
	for method, v := range rewrite {
		method = strings.ToUpper(method)
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			continue
		}
		old2new := make(map[string]string)
		for ourl, nurl := range v {
			ourl = strings.TrimSpace(ourl)
			if ourl == "" {
				continue
			}
			nurl = strings.TrimSpace(nurl)
			if nurl == "" {
				continue
			}
			ourl = cleanPath(ourl)
			nurl = cleanPath(nurl)
			old2new[ourl] = nurl
		}
		if len(old2new) > 0 {
			tmp[method] = old2new
		}
	}
	old := r.handlerrewrite
	r.handlerrewrite = tmp
	if r.status {
		var get, post, put, patch, delete bool
		if len(old) > 0 {
			oldv, ok1 := old[http.MethodGet]
			newv, ok2 := r.handlerrewrite[http.MethodGet]
			var eo map[string]string
			if ok1 {
				eo = make(map[string]string)
				for ourl, nurl := range oldv {
					if _, ok := r.tmpget[nurl]; ok {
						eo[ourl] = nurl
					}
				}
			}
			var en map[string]string
			if ok2 {
				en = make(map[string]string)
				for ourl, nurl := range newv {
					if _, ok := r.tmpget[nurl]; ok {
						en[ourl] = nurl
					}
				}
			}
			if !maps.Equal(eo, en) {
				get = true
			}
			oldv, ok1 = old[http.MethodPost]
			newv, ok2 = r.handlerrewrite[http.MethodPost]
			if ok1 {
				eo = make(map[string]string)
				for ourl, nurl := range oldv {
					if _, ok := r.tmppost[nurl]; ok {
						eo[ourl] = nurl
					}
				}
			} else {
				eo = nil
			}
			if ok2 {
				en = make(map[string]string)
				for ourl, nurl := range newv {
					if _, ok := r.tmppost[nurl]; ok {
						en[ourl] = nurl
					}
				}
			} else {
				en = nil
			}
			if !maps.Equal(eo, en) {
				post = true
			}
			oldv, ok1 = old[http.MethodPut]
			newv, ok2 = r.handlerrewrite[http.MethodPut]
			if ok1 {
				eo = make(map[string]string)
				for ourl, nurl := range oldv {
					if _, ok := r.tmpput[nurl]; ok {
						eo[ourl] = nurl
					}
				}
			} else {
				eo = nil
			}
			if ok2 {
				en = make(map[string]string)
				for ourl, nurl := range newv {
					if _, ok := r.tmpput[nurl]; ok {
						en[ourl] = nurl
					}
				}
			} else {
				en = nil
			}
			if !maps.Equal(eo, en) {
				put = true
			}
			oldv, ok1 = old[http.MethodPatch]
			newv, ok2 = r.handlerrewrite[http.MethodPatch]
			if ok1 {
				eo = make(map[string]string)
				for ourl, nurl := range oldv {
					if _, ok := r.tmppatch[nurl]; ok {
						eo[ourl] = nurl
					}
				}
			} else {
				eo = nil
			}
			if ok2 {
				en = make(map[string]string)
				for ourl, nurl := range newv {
					if _, ok := r.tmppatch[nurl]; ok {
						en[ourl] = nurl
					}
				}
			} else {
				en = nil
			}
			if !maps.Equal(eo, en) {
				patch = true
			}
			oldv, ok1 = old[http.MethodDelete]
			newv, ok2 = r.handlerrewrite[http.MethodDelete]
			if ok1 {
				eo = make(map[string]string)
				for ourl, nurl := range oldv {
					if _, ok := r.tmpdelete[nurl]; ok {
						eo[ourl] = nurl
					}
				}
			} else {
				eo = nil
			}
			if ok2 {
				en = make(map[string]string)
				for ourl, nurl := range newv {
					if _, ok := r.tmpdelete[nurl]; ok {
						en[ourl] = nurl
					}
				}
			} else {
				en = nil
			}
			if !maps.Equal(eo, en) {
				delete = true
			}
		} else {
			for method, v := range r.handlerrewrite {
				switch method {
				case http.MethodGet:
					for _, nurl := range v {
						if _, get = r.tmpget[nurl]; get {
							break
						}
					}
				case http.MethodPost:
					for _, nurl := range v {
						if _, post = r.tmppost[nurl]; post {
							break
						}
					}
				case http.MethodPut:
					for _, nurl := range v {
						if _, put = r.tmpput[nurl]; put {
							break
						}
					}
				case http.MethodPatch:
					for _, nurl := range v {
						if _, patch = r.tmppatch[nurl]; patch {
							break
						}
					}
				case http.MethodDelete:
					for _, nurl := range v {
						if _, delete = r.tmpdelete[nurl]; delete {
							break
						}
					}
				}
			}
		}
		if get {
			r.rebuildget()
		}
		if post {
			r.rebuildpost()
		}
		if put {
			r.rebuildput()
		}
		if patch {
			r.rebuildpatch()
		}
		if delete {
			r.rebuilddelete()
		}
	}
}

// first key path,second method,value timeout(if timeout <= 0 means no timeout)
func (r *Router) UpdateHandlerTimeout(timeout map[string]map[string]time.Duration) {
	r.lker.Lock()
	defer r.lker.Unlock()
	tmp := make(map[string]map[string]time.Duration)
	for method, v := range timeout {
		method = strings.ToUpper(method)
		if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
			continue
		}
		vv := make(map[string]time.Duration)
		tmp[method] = vv
		for url, t := range v {
			url = cleanPath(url)
			vv[url] = t
		}
	}
	old := r.handlertimeout
	r.handlertimeout = tmp
	if len(old) > 0 {
		//remove or replace timeout on the handler
		for method, otimeout := range old {
			var hs map[string]*handler
			switch method {
			case http.MethodGet:
				hs = r.tmpget
			case http.MethodPost:
				hs = r.tmppost
			case http.MethodPut:
				hs = r.tmpput
			case http.MethodPatch:
				hs = r.tmppatch
			case http.MethodDelete:
				hs = r.tmpdelete
			}
			ntimeout, ok := r.handlertimeout[method]
			if !ok {
				//remove all old timeout
				for path := range otimeout {
					if h, ok := hs[path]; ok {
						h.timeout.Store(r.s.c.DefaultHandlerTimeout.StdDuration().Nanoseconds())
					}
				}
			} else {
				for path, oduration := range otimeout {
					if nduration, ok := ntimeout[path]; !ok {
						//remove
						if h, ok := hs[path]; ok {
							h.timeout.Store(r.s.c.DefaultHandlerTimeout.StdDuration().Nanoseconds())
						}
					} else if oduration != nduration {
						//replace
						if h, ok := hs[path]; ok {
							//specific handler timeout has high priority then DefaultHandlerTimeout
							h.timeout.Store(nduration.Nanoseconds())
						}
					} else {
						//skip
					}
				}
			}
		}
	}
	//add new timeout
	for method, timeout := range r.handlertimeout {
		var hs map[string]*handler
		switch method {
		case http.MethodGet:
			hs = r.tmpget
		case http.MethodPost:
			hs = r.tmppost
		case http.MethodPut:
			hs = r.tmpput
		case http.MethodPatch:
			hs = r.tmppatch
		case http.MethodDelete:
			hs = r.tmpdelete
		}
		for url, duration := range timeout {
			if h, ok := hs[url]; ok {
				//specific handler timeout has high priority then DefaultHandlerTimeout
				h.timeout.Store(duration.Nanoseconds())
			}
		}
	}
}

func (r *Router) rebuild() {
	r.lker.Lock()
	defer r.lker.Unlock()
	r.status = true
	r.rebuildget()
	r.rebuildpost()
	r.rebuildput()
	r.rebuildpatch()
	r.rebuilddelete()
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

// Warning!this must be called before Get,Post,Put,Patch,Delete
func (r *Router) Use(globalMids ...OutsideHandler) {
	r.lker.Lock()
	r.globalmids = append(r.globalmids, globalMids...)
	r.lker.Unlock()
}

func (r *Router) rebuildget() {
	tmpget := maps.Clone(r.tmpget)
	for path := range tmpget {
		slog.Info("[web.server] handler",
			slog.String("method", http.MethodGet),
			slog.String("path", path))
	}
	for ourl, nurl := range r.handlerrewrite[http.MethodGet] {
		h, ok := tmpget[nurl]
		if ok {
			tmpget[ourl] = h
			slog.Info("[web.server] rewriter",
				slog.String("method", http.MethodGet),
				slog.String("opath", ourl),
				slog.String("npath", nurl))
		}
	}
	r.get.Store(&tmpget)
}

func (r *Router) Get(path string, ServerSentEvent bool, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	r.tmpget[path] = r.insideHandler(http.MethodGet, path, handlers, ServerSentEvent)
	if r.status {
		r.rebuildget()
	}
}

func (r *Router) UnregisterGet(path string) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	if _, ok := r.tmpget[path]; ok {
		delete(r.tmpget, path)
		if r.status {
			r.rebuildget()
		}
	}
}

func (r *Router) rebuildpost() {
	tmppost := maps.Clone(r.tmppost)
	for path := range tmppost {
		slog.Info("[web.server] handler",
			slog.String("metod", http.MethodPost),
			slog.String("path", path))
	}
	for ourl, nurl := range r.handlerrewrite[http.MethodPost] {
		if h, ok := tmppost[nurl]; ok {
			tmppost[ourl] = h
			slog.Info("[web.server] rewriter",
				slog.String("method", http.MethodPost),
				slog.String("opath", ourl),
				slog.String("npath", nurl))
		}
	}
	r.post.Store(&tmppost)
}

func (r *Router) Post(path string, ServerSentEvent bool, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	r.tmppost[path] = r.insideHandler(http.MethodPost, path, handlers, ServerSentEvent)
	if r.status {
		r.rebuildpost()
	}
}

func (r *Router) UnregisterPost(path string) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	if _, ok := r.tmppost[path]; ok {
		delete(r.tmppost, path)
		if r.status {
			r.rebuildpost()
		}
	}
}

func (r *Router) rebuildpatch() {
	tmppatch := maps.Clone(r.tmppatch)
	for path := range tmppatch {
		slog.Info("[web.server] handler",
			slog.String("metod", http.MethodPatch),
			slog.String("path", path))
	}
	for ourl, nurl := range r.handlerrewrite[http.MethodPatch] {
		if h, ok := tmppatch[nurl]; ok {
			tmppatch[ourl] = h
			slog.Info("[web.server] rewriter",
				slog.String("method", http.MethodPatch),
				slog.String("opath", ourl),
				slog.String("npath", nurl))
		}
	}
	r.patch.Store(&tmppatch)
}

func (r *Router) Patch(path string, ServerSentEvent bool, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	r.tmppatch[path] = r.insideHandler(http.MethodPatch, path, handlers, ServerSentEvent)
	if r.status {
		r.rebuildpatch()
	}
}

func (r *Router) UnregisterPatch(path string) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	if _, ok := r.tmppatch[path]; ok {
		delete(r.tmppatch, path)
		if r.status {
			r.rebuildpatch()
		}
	}
}

func (r *Router) rebuildput() {
	tmpput := maps.Clone(r.tmpput)
	for path := range tmpput {
		slog.Info("[web.server] handler",
			slog.String("metod", http.MethodPut),
			slog.String("path", path))
	}
	for ourl, nurl := range r.handlerrewrite[http.MethodPut] {
		if h, ok := tmpput[nurl]; ok {
			tmpput[ourl] = h
			slog.Info("[web.server] rewriter",
				slog.String("method", http.MethodPut),
				slog.String("opath", ourl),
				slog.String("npath", nurl))
		}
	}
	r.put.Store(&tmpput)
}

func (r *Router) Put(path string, ServerSentEvent bool, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	r.tmpput[path] = r.insideHandler(http.MethodPut, path, handlers, ServerSentEvent)
	if r.status {
		r.rebuildput()
	}
}

func (r *Router) UnregisterPut(path string) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	if _, ok := r.tmpput[path]; ok {
		delete(r.tmpput, path)
		if r.status {
			r.rebuildput()
		}
	}
}

func (r *Router) rebuilddelete() {
	tmpdelete := maps.Clone(r.tmpdelete)
	for path := range tmpdelete {
		slog.Info("[web.server] handler",
			slog.String("metod", http.MethodPut),
			slog.String("path", path))
	}
	for ourl, nurl := range r.handlerrewrite[http.MethodDelete] {
		if h, ok := tmpdelete[nurl]; ok {
			tmpdelete[ourl] = h
			slog.Info("[web.server] rewriter",
				slog.String("method", http.MethodDelete),
				slog.String("opath", ourl),
				slog.String("npath", nurl))
		}
	}
	r.delete.Store(&tmpdelete)
}

func (r *Router) Delete(path string, ServerSentEvent bool, handlers ...OutsideHandler) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	r.tmpdelete[path] = r.insideHandler(http.MethodDelete, path, handlers, ServerSentEvent)
	if r.status {
		r.rebuilddelete()
	}
}

func (r *Router) UnregisterDelete(path string) {
	path = cleanPath(path)
	r.lker.Lock()
	defer r.lker.Unlock()
	if _, ok := r.tmpdelete[path]; ok {
		delete(r.tmpdelete, path)
		if r.status {
			r.rebuilddelete()
		}
	}
}

func (r *Router) insideHandler(method, path string, handlers []OutsideHandler, sse bool) *handler {
	h := &handler{r: r, method: method, path: path, sse: sse}
	h.totalhandlers = make([]OutsideHandler, len(r.globalmids)+len(handlers))
	copy(h.totalhandlers, r.globalmids)
	copy(h.totalhandlers[len(r.globalmids):], handlers)
	if r.handlertimeout == nil {
		if r.s.c.DefaultHandlerTimeout > 0 {
			h.timeout.Store(r.s.c.DefaultHandlerTimeout.StdDuration().Nanoseconds())
		}
	} else if timeout, ok := r.handlertimeout[method]; ok {
		if duration, ok := timeout[path]; ok {
			h.timeout.Store(duration.Nanoseconds())
		} else if r.s.c.DefaultHandlerTimeout > 0 {
			h.timeout.Store(r.s.c.DefaultHandlerTimeout.StdDuration().Nanoseconds())
		}
	} else if r.s.c.DefaultHandlerTimeout > 0 {
		h.timeout.Store(r.s.c.DefaultHandlerTimeout.StdDuration().Nanoseconds())
	}
	return h
}
func (r *Router) notFoundHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write(common.STB(cerror.ErrNotExist.Json()))
	slog.Error("[web.server] path not exist",
		slog.String("cip", realip(req)),
		slog.String("path", req.URL.Path),
		slog.String("method", req.Method))
}
func (r *Router) srcFileHandler(resp http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	if path == "/" || path == "" {
		path = "index.html"
	} else if path[0] == '/' {
		path = path[1:]
	}
	if !strings.HasSuffix(path, ".gz") && strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		//try to serve the pre gzip compressed file
		//if pre gzip compressed file not exist,we fallback to serve the normal file
		//if the path to pre gzip compressed file is a dir,we fallback to serve the normal file
		tmppath := path + ".gz"
		if file, e := r.srcroot.Open(tmppath); e == nil {
			if fileinfo, e := file.Stat(); e != nil {
				resp.Header().Set("Content-Type", "application/json")
				resp.WriteHeader(int(cerror.ErrSystem.GetHttpcode()))
				resp.Write(common.STB(cerror.ErrSystem.Json()))
				slog.Error("[web.server] get static src file info failed",
					slog.String("cip", realip(req)),
					slog.String("path", tmppath),
					slog.String("method", req.Method),
					slog.String("error", e.Error()))
				file.Close()
				return
			} else if fileinfo.Mode().IsRegular() {
				resp.Header().Set("Content-Length", strconv.FormatInt(fileinfo.Size(), 10))
				resp.Header().Set("Content-Encoding", "gzip")
				index := strings.LastIndex(path, "/")
				if index == -1 {
					http.ServeContent(resp, req, path, fileinfo.ModTime(), file.(io.ReadSeeker))
				} else {
					http.ServeContent(resp, req, path[index+1:], fileinfo.ModTime(), file.(io.ReadSeeker))
				}
				file.Close()
				return
			}
			file.Close()
		} else if !os.IsNotExist(e) {
			resp.Header().Set("Content-Type", "application/json")
			resp.WriteHeader(int(cerror.ErrSystem.GetHttpcode()))
			resp.Write(common.STB(cerror.ErrSystem.Json()))
			slog.Error("[web.server] open static src file failed",
				slog.String("cip", realip(req)),
				slog.String("path", tmppath),
				slog.String("method", req.Method),
				slog.String("error", e.Error()))
			return
		}
	}
	if file, e := r.srcroot.Open(path); e != nil {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrSystem.GetHttpcode()))
		resp.Write(common.STB(cerror.ErrSystem.Json()))
		slog.Error("[web.server] open static src file failed",
			slog.String("cip", realip(req)),
			slog.String("path", path),
			slog.String("method", req.Method),
			slog.String("error", e.Error()))
	} else if fileinfo, e := file.Stat(); e != nil {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrSystem.GetHttpcode()))
		resp.Write(common.STB(cerror.ErrSystem.Json()))
		slog.Error("[web.server] get static src file info failed",
			slog.String("cip", realip(req)),
			slog.String("path", path),
			slog.String("method", req.Method),
			slog.String("error", e.Error()))
		file.Close()
	} else if !fileinfo.Mode().IsRegular() {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrNotExist.GetHttpcode()))
		resp.Write(common.STB(cerror.ErrNotExist.Json()))
		slog.Error("[web.server] static src file not exist",
			slog.String("cip", realip(req)),
			slog.String("path", path),
			slog.String("method", req.Method))
		file.Close()
	} else {
		http.ServeContent(resp, req, fileinfo.Name(), fileinfo.ModTime(), file.(io.ReadSeeker))
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
	resp.Header().Set("Access-Control-Allow-Origin", r.cors(origin))
	if resp.Header().Get("Access-Control-Allow-Origin") == "" {
		resp.WriteHeader(http.StatusForbidden)
		slog.Error("[web.server] cors check failed",
			slog.String("cip", realip(req)),
			slog.String("path", req.URL.Path),
			slog.String("method", req.Method))
		return
	}
	if r.s.c.CorsAllowCredentials {
		resp.Header().Set("Access-Control-Allow-Credentials", "true")
		resp.Header().Set("Access-Control-Allow-Origin", origin)
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
}
func (r *Router) corsNormal(resp http.ResponseWriter, req *http.Request) bool {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	resp.Header().Add("Vary", "Origin")
	resp.Header().Set("Access-Control-Allow-Origin", r.cors(origin))
	if resp.Header().Get("Access-Control-Allow-Origin") == "" {
		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(int(cerror.ErrCors.GetHttpcode()))
		resp.Write(common.STB(cerror.ErrCors.Json()))
		slog.Error("[web.server] cors check failed",
			slog.String("cip", realip(req)),
			slog.String("path", req.URL.Path),
			slog.String("method", req.Method))
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
func (r *Router) cors(origin string) string {
	cutdefaultport := -1
	for _, v := range r.s.c.CorsAllowedOrigins {
		if v == "*" {
			return "*"
		} else if strings.Contains(v, "*") {
			switch v {
			case "http://*":
				if strings.HasPrefix(origin, "http://") {
					return origin
				}
			case "https://*":
				if strings.HasPrefix(origin, "https://") {
					return origin
				}
			default:
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
				if cutdefaultport < 0 {
					if strings.HasPrefix(origin, "http://") && origin[len(origin)-1] != ']' && strings.HasSuffix(origin, ":80") {
						cutdefaultport = 3
					} else if strings.HasPrefix(origin, "https://") && origin[len(origin)-1] != ']' && strings.HasSuffix(origin, ":443") {
						cutdefaultport = 4
					} else {
						cutdefaultport = 0
					}
				}
				if index == len(origin)-cutdefaultport {
					return origin
				}
			}
		} else if v == origin {
			return origin
		} else {
			if cutdefaultport < 0 {
				if strings.HasPrefix(origin, "http://") && origin[len(origin)-1] != ']' && strings.HasSuffix(origin, ":80") {
					cutdefaultport = 3
				} else if strings.HasPrefix(origin, "https://") && origin[len(origin)-1] != ']' && strings.HasSuffix(origin, ":443") {
					cutdefaultport = 4
				} else {
					cutdefaultport = 0
				}
			}
			if cutdefaultport > 0 && origin[:len(origin)-cutdefaultport] == v {
				return origin
			}
		}
	}
	return ""
}
func (r *Router) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodOptions {
		r.corsOptions(resp, req)
		return
	}
	if !r.corsNormal(resp, req) {
		return
	}
	var hs map[string]*handler
	switch req.Method {
	case http.MethodGet:
		hs = *r.get.Load()
	case http.MethodPost:
		hs = *r.post.Load()
	case http.MethodPut:
		hs = *r.put.Load()
	case http.MethodPatch:
		hs = *r.patch.Load()
	case http.MethodDelete:
		hs = *r.delete.Load()
	}
	if hs != nil {
		h, ok := hs[req.URL.Path]
		if !ok {
			req.URL.Path = cleanPath(req.URL.Path)
			h, ok = hs[req.URL.Path]
		}
		if ok {
			h.handle(resp, req)
			return
		}
	}
	if r.srcroot == nil || req.Method != http.MethodGet {
		r.notFoundHandler(resp, req)
		return
	}
	r.srcFileHandler(resp, req)
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
	buf := make([]byte, len(origin)+1)
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
	return common.BTS(buf[:realpos])
}
