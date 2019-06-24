package nobody

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	c *Config
	r *Router
	b []HandleFunc
	a []HandleFunc
	s *http.Server
	w *sync.WaitGroup
}

//new server and router
func New(c *Config) (*Server, *Router) {
	if c == nil {
		panic("config is nil")
	}
	//init
	instance := &Server{
		c: c,
		r: new(Router),
		b: make([]HandleFunc, 0),
		a: make([]HandleFunc, 0),
		s: &http.Server{},
		//d: false,
		w: new(sync.WaitGroup),
	}
	instance.r.init()
	instance.s.Handler = instance
	return instance, instance.r
}

//start http server with the config
func (s *Server) Start() {
	//config
	s.s.Addr = s.c.Addr
	s.s.SetKeepAlivesEnabled(s.c.KeepAlive)
	s.s.ReadHeaderTimeout = time.Duration(s.c.ReadHeaderTimeout) * time.Millisecond
	s.s.ReadTimeout = time.Duration(s.c.ReadTimeout) * time.Millisecond
	s.s.WriteTimeout = time.Duration(s.c.WriteTimeout) * time.Millisecond
	s.s.IdleTimeout = time.Duration(s.c.IdleTimeout) * time.Millisecond
	if s.c.MaxHeaderBytes == 0 {
		s.c.MaxHeaderBytes = 1024
	}
	s.s.MaxHeaderBytes = s.c.MaxHeaderBytes
	//chan
	done := make(chan bool, 1)
	//start
	go func() {
		if s.c.CertFilePath != "" && s.c.KeyFilePath != "" {
			s.s.ListenAndServeTLS(s.c.CertFilePath, s.c.KeyFilePath)
		} else {
			s.s.ListenAndServe()
		}
		done <- true
	}()
	fmt.Println("Welcome to use 'nobody' web frame!")
	<-done
	s.w.Wait()
	return
}

//when client connection state change,this function will be called
func (s *Server) RegisterConnStateFunc(f ConnStateFunc) {
	s.s.ConnState = f
}

//when shutdown the server,all registered functions will be called
func (s *Server) RegisterShutdownFunc(f ShutdownFunc) {
	s.w.Add(1)
	s.s.RegisterOnShutdown(func() {
		f()
		s.w.Done()
	})
}

//shutdown server gracefully
func (s *Server) Shutdown() error {
	fmt.Println("Server is shutting down,it may take some time because it is graceful!")
	return s.s.Shutdown(context.Background())
}

//register global midware
func (s *Server) GlobalBefore(handler ...HandleFunc) {
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		s.b = append(s.b, temp...)
	}
}

//register global midware
func (s *Server) GlobalAfter(handler ...HandleFunc) {
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		s.a = append(s.a, temp...)
	}
}

//deal request
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := poolInstance.getContext()
	ctx.request = r
	ctx.response = w
	//before
	for _, f := range s.b {
		f(ctx)
	}
	//current
	s.r.search(ctx)
	if ctx.calls == nil || len(ctx.calls) == 0 {
		//didn't match
		http.NotFound(w, r)
		goto end
	}
	for _, f := range ctx.calls {
		//match
		f(ctx)
	}
	//after
	for _, f := range s.a {
		f(ctx)
	}
end:
	go poolInstance.putContext(ctx)
}
