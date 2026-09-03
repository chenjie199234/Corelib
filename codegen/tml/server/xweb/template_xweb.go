package xweb

import (
	"os"
	"text/template"
)

const txt = `package xweb

import (
	"crypto/tls"
	"log/slog"
	"sync/atomic"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/web/mids"
)

var s atomic.Pointer[web.WebServer]
var r atomic.Pointer[web.Router]

func StartWebServer() {
	c := config.GetWebServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				slog.Error("[xweb] load cert failed:", slog.String("cert", cert), slog.String("key", key), slog.String("error", e.Error()))
				return
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	server, e := web.NewWebServer(c.ServerConfig, tlsc)
	if e != nil {
		slog.Error("[xweb] new server failed", slog.String("error", e.Error()))
		return
	}
	s.Store(server)

	router, e := server.NewRouter()
	if e != nil {
		slog.Error("[xweb] new router failed", slog.String("error", e.Error()))
		return
	}
	r.Store(router)
	UpdateHandlerTimeout(config.AC.HandlerTimeout)
	UpdateWebPathRewrite(config.AC.WebPathRewrite)

	//this place can register global midwares
	//router.Use(globalmidwares)

	//example
	//api.RegisterExampleWebServer(router, service.SvcExample, mids.AllMids())
	//you need to register your service here
	api.RegisterStatusWebServer(router, service.SvcStatus, mids.AllMids())

	server.SetRouter(router)
	if e = server.StartWebServer(":8000"); e != nil && e != web.ErrServerClosed {
		slog.Error("[xweb] start server failed", slog.String("error", e.Error()))
		return
	}
	slog.Info("[xweb] server closed")
}

// first key:path,second key:method
func UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmpr := r.Load()
	if tmpr != nil {
		tmpr.UpdateHandlerTimeout(timeout)
	}
}

// first key:method,second key:origin url,value:new url
func UpdateWebPathRewrite(rewrite map[string]map[string]string) {
	tmpr := r.Load()
	if tmpr != nil {
		tmpr.UpdateHandlerRewrite(rewrite)
	}
}

func StopWebServer(force bool) {
	tmps := s.Load()
	if tmps != nil {
		tmps.StopWebServer(force)
	}
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./server/xweb/", 0755); e != nil {
		panic("mkdir ./server/xweb/ error: " + e.Error())
	}
	xwebtemplate, e := template.New("./server/xweb/xweb.go").Parse(txt)
	if e != nil {
		panic("parse ./server/xweb/xweb.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./server/xweb/xweb.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./server/xweb/xweb.go error: " + e.Error())
	}
	if e := xwebtemplate.Execute(file, packagename); e != nil {
		panic("write ./server/xweb/xweb.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./server/xweb/xweb.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./server/xweb/xweb.go error: " + e.Error())
	}
}
