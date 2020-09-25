package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
	"unsafe"

	"github.com/julienschmidt/httprouter"
)

type Web struct {
	conf        *WebConfig
	server      *http.Server
	router      *httprouter.Router
	contextpool *sync.Pool
}

type cancelfunckey struct{}

func NewInstance(c *WebConfig) *Web {
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
	instance.server.ConnContext = func(ctx context.Context, conn net.Conn) context.Context {
		conn.(*net.TCPConn).SetReadBuffer(c.SocketReadBufferLen)
		conn.(*net.TCPConn).SetWriteBuffer(c.SocketWriteBufferLen)
		if c.WriteTimeout != 0 {
			ctx, f := context.WithTimeout(ctx, time.Duration(c.WriteTimeout)*time.Millisecond)
			ctx = context.WithValue(ctx, cancelfunckey{}, f)
			return ctx
		} else if c.ReadTimeout != 0 {
			ctx, f := context.WithTimeout(ctx, time.Duration(c.ReadTimeout)*time.Millisecond)
			ctx = context.WithValue(ctx, cancelfunckey{}, f)
			return ctx
		} else {
			ctx, f := context.WithTimeout(ctx, 200*time.Millisecond)
			ctx = context.WithValue(ctx, cancelfunckey{}, f)
			return ctx
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

func (this *Web) insideHandler(handlers ...OutsideHandler) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ctx := this.getContext(w, r, p)
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
		tempf := ctx.Value(cancelfunckey{})
		if tempf != nil {
			f, ok := tempf.(context.CancelFunc)
			if ok {
				f()
			}
		}
		this.putContext(ctx)
	}
}
func (this *Web) GET(path string, handlers ...OutsideHandler) {
	this.router.GET(path, this.insideHandler(handlers...))
}
func (this *Web) POST(path string, handlers ...OutsideHandler) {
	this.router.POST(path, this.insideHandler(handlers...))
}
func (this *Web) PUT(path string, handlers ...OutsideHandler) {
	this.router.PUT(path, this.insideHandler(handlers...))
}
func (this *Web) PATCH(path string, handlers ...OutsideHandler) {
	this.router.PATCH(path, this.insideHandler(handlers...))
}
func (this *Web) DELETE(path string, handlers ...OutsideHandler) {
	this.router.DELETE(path, this.insideHandler(handlers...))
}
func (this *Web) HEAD(path string, handlers ...OutsideHandler) {
	this.router.HEAD(path, this.insideHandler(handlers...))
}
func (this *Web) OPTIONS(path string, handlers ...OutsideHandler) {
	this.router.OPTIONS(path, this.insideHandler(handlers...))
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

func Str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func Byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
