package xweb

import (
	"os"
	"text/template"
)

const txt = `package xweb

import (
	"crypto/tls"
	"log/slog"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/web/mids"
)

var s *web.WebServer

// StartWebServer -
func StartWebServer() {
	c := config.GetWebServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				slog.ErrorContext(nil, "[xweb] load cert failed:", slog.String("cert", cert), slog.String("key", key), slog.String("error",e.Error()))
				return 
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	var e error
	if s, e = web.NewWebServer(c.ServerConfig, model.Project, model.Group, model.Name, tlsc); e != nil {
		slog.ErrorContext(nil, "[xweb] new server failed", slog.String("error",e.Error()))
		return
	}
	UpdateHandlerTimeout(config.AC.HandlerTimeout)
	UpdateWebPathRewrite(config.AC.WebPathRewrite)

	r := s.NewRouter()

	//this place can register global midwares
	//r.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusWebServer(r, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleWebServer(r, service.SvcExample, mids.AllMids())

	s.SetRouter(r)
	if e = s.StartWebServer(":8000"); e != nil && e != web.ErrServerClosed {
		slog.ErrorContext(nil, "[xweb] start server failed", slog.String("error",e.Error()))
		return
	}
	slog.InfoContext(nil, "[xweb] server closed")
}

// UpdateHandlerTimeout -
// first key path,second key method,value timeout duration
func UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	if s != nil {
		s.UpdateHandlerTimeout(timeout)
	}
}

// UpdateWebPathRewrite -
// first key method,second key origin url,value rewrite url
func UpdateWebPathRewrite(rewrite map[string]map[string]string) {
	if s != nil {
		s.UpdateHandlerRewrite(rewrite)
	}
}

// StopWebServer force - false(graceful),true(not graceful)
func StopWebServer(force bool) {
	if s != nil {
		s.StopWebServer(force)
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
