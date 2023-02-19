package xweb

import (
	"os"
	"text/template"
)

const txt = `package xweb

import (
	"net/http"
	"strings"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/web/mids"
)

var s *web.WebServer

// StartWebServer -
func StartWebServer() {
	c := config.GetWebServerConfig()
	webc := &web.ServerConfig{
		WaitCloseMode:  c.CloseMode,
		ConnectTimeout: time.Duration(c.ConnectTimeout),
		GlobalTimeout:  time.Duration(c.GlobalTimeout),
		IdleTimeout:    time.Duration(c.IdleTimeout),
		HeartProbe:     time.Duration(c.HeartProbe),
		SrcRoot:        c.SrcRoot,
		MaxHeader:      2048,
		Certs:          c.Certs,
	}
	if c.Cors != nil {
		webc.Cors = &web.CorsConfig{
			AllowedOrigin:    c.Cors.CorsOrigin,
			AllowedHeader:    c.Cors.CorsHeader,
			ExposeHeader:     c.Cors.CorsExpose,
			AllowCredentials: true,
			MaxAge:           24 * time.Hour,
		}
	}
	var e error
	if s, e = web.NewWebServer(webc, model.Group, model.Name); e != nil {
		log.Error(nil, "[xweb] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC.HandlerTimeout)
	UpdateWebPathRewrite(config.AC.WebPathRewrite)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusWebServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleWebServer(s, service.SvcExample, mids.AllMids())

	if e = s.StartWebServer(":8000"); e != nil && e != web.ErrServerClosed {
		log.Error(nil, "[xweb] start error:", e)
		return
	}
	log.Info(nil, "[xweb] server closed")
}

// UpdateHandlerTimeout -
// first key path,second key method,value timeout duration
func UpdateHandlerTimeout(hts map[string]map[string]ctime.Duration) {
	if s == nil {
		return
	}
	cc := make(map[string]map[string]time.Duration)
	for path, methods := range hts {
		for method, timeout := range methods {
			method = strings.ToUpper(method)
			if method != http.MethodGet && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
				continue
			}
			if _, ok := cc[method]; !ok {
				cc[method] = make(map[string]time.Duration)
			}
			cc[method][path] = timeout.StdDuration()
		}
	}
	s.UpdateHandlerTimeout(cc)
}

// UpdateWebPathRewrite -
//key origin url,value rewrite url
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
