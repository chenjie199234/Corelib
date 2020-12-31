package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	//"github.com/chenjie199234/Corelib/sys/trace"
	"github.com/julienschmidt/httprouter"
)

type Web struct {
	conf        *WebConfig
	server      *http.Server
	router      *httprouter.Router
	contextpool *sync.Pool
}

func NewInstance(c *WebConfig) *Web {
	checkconfig(c)
	instance := &Web{
		conf: c,
		server: &http.Server{
			Addr:              c.Addr,
			ReadTimeout:       time.Duration(c.ReadTimeout) * time.Millisecond,
			ReadHeaderTimeout: time.Duration(c.ReadHeaderTimeout) * time.Millisecond,
			WriteTimeout:      time.Duration(c.WriteTimeout) * time.Millisecond,
			IdleTimeout:       time.Duration(c.IdleTimeout) * time.Millisecond,
			MaxHeaderBytes:    c.MaxHeaderBytes,
		},
		router: httprouter.New(),
		contextpool: &sync.Pool{
			New: func() interface{} {
				return &Context{}
			},
		},
	}
	instance.server.ConnState = func(conn net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conn.(*net.TCPConn).SetReadBuffer(c.SocketReadBufferLen)
			conn.(*net.TCPConn).SetWriteBuffer(c.SocketWriteBufferLen)
		}
	}
	instance.router.PanicHandler = func(w http.ResponseWriter, r *http.Request, i interface{}) {
		fmt.Printf("[Web.PanicHandler]panic:\n,%s\n", i)
		http.Error(w, "500 server error:panic", http.StatusInternalServerError)
	}
	return instance
}
func (this *Web) getContext(w http.ResponseWriter, r *http.Request, p httprouter.Params) *Context {
	ctx := this.contextpool.Get().(*Context)
	ctx.w = w
	ctx.r = r
	ctx.p = p
	ctx.s = true
	ctx.Context = r.Context()
	return ctx
}
func (this *Web) putContext(ctx *Context) {
	ctx.w = nil
	ctx.r = nil
	ctx.p = nil
	ctx.s = false
	ctx.Context = nil
	this.contextpool.Put(ctx)
}

//return code message data
type OutsideHandler func(*Context)

var emptydata = []byte{'{', '}'}

func (this *Web) insideHandler(timeout int, handlers ...OutsideHandler) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ctx := this.getContext(w, r, p)
		now := time.Now()
		var dl time.Time
		var clientdeadline int64
		if temp := ctx.GetHeader("Deadline"); temp != "" {
			var e error
			clientdeadline, e = strconv.ParseInt(temp, 10, 64)
			if e != nil {
				fmt.Printf("[Web.insideHandler]parse client deadline error:%s\n", e)
				ctx.w.WriteHeader(http.StatusBadRequest)
				ctx.w.Write(emptydata)
			} else if clientdeadline != 0 {
				if clientdeadline <= now.UnixNano()+int64(time.Millisecond) {
					ctx.w.WriteHeader(http.StatusRequestTimeout)
					ctx.w.Write(emptydata)
					return
				}
			}
		}
		var serverdeadline int64
		if timeout > 0 && timeout < this.conf.WriteTimeout {
			serverdeadline = now.UnixNano() + int64(timeout)*int64(time.Millisecond)
		} else if this.conf.WriteTimeout > 0 {
			serverdeadline = now.UnixNano() + int64(this.conf.WriteTimeout)*int64(time.Millisecond)
		}
		if clientdeadline != 0 && serverdeadline != 0 {
			if clientdeadline < serverdeadline {
				dl = time.Unix(0, clientdeadline)
			} else {
				dl = time.Unix(0, serverdeadline)
			}
		} else if clientdeadline != 0 {
			dl = time.Unix(0, clientdeadline)
		} else if serverdeadline != 0 {
			dl = time.Unix(0, serverdeadline)
		}

		if !dl.IsZero() {
			var f context.CancelFunc
			ctx.Context, f = context.WithDeadline(ctx.Context, dl)
			defer f()
		}
		//set traceid
		//ctx.Context = trace.SetTrace(ctx.Context, trace.MakeTrace())
		//deal logic
		for _, handler := range handlers {
			handler(ctx)
			if !ctx.s {
				break
			}
		}
		if ctx.s {
			ctx.w.WriteHeader(http.StatusOK)
			ctx.w.Write(emptydata)
		}
		this.putContext(ctx)
	}
}

func (this *Web) GET(path string, timeout int, handlers ...OutsideHandler) {
	this.router.GET(path, this.insideHandler(timeout, handlers...))
}
func (this *Web) POST(path string, timeout int, handlers ...OutsideHandler) {
	this.router.POST(path, this.insideHandler(timeout, handlers...))
}
func (this *Web) ServeFiles(path string, root http.FileSystem) {
	this.router.ServeFiles(path, root)
}

func (this *Web) StartWebServer() {
	this.server.Handler = this.router
	if this.conf.TlsCertFile != "" && this.conf.TlsKeyFile != "" {
		this.server.ListenAndServeTLS(this.conf.TlsCertFile, this.conf.TlsKeyFile)
	} else {
		this.server.ListenAndServe()
	}
}

func (this *Web) Shudown() {
	this.server.Shutdown(context.Background())
}
