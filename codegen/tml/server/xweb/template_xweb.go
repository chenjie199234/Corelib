package xweb

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xweb

import (
	"net/http"
	"strings"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/web/mids"
)

var s *web.WebServer

//StartWebServer -
func StartWebServer() {
	c := config.GetWebServerConfig()
	webc := &web.ServerConfig{
		GlobalTimeout:      time.Duration(c.GlobalTimeout),
		IdleTimeout:        time.Duration(c.IdleTimeout),
		HeartProbe:         time.Duration(c.HeartProbe),
		StaticFileRootPath: c.StaticFilePath,
		MaxHeader:          1024,
		SocketRBuf:         2048,
		SocketWBuf:         2048,
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
	if s, e = web.NewWebServer(webc, api.Group, api.Name); e != nil {
		log.Error(nil,"[xweb] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusWebServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleWebServer(s, service.SvcExample, mids.AllMids())

	if e = s.StartWebServer(":8000"); e != nil {
		if e != web.ErrServerClosed {
			log.Error(nil,"[xweb] start error:", e)
		} else {
			log.Info(nil,"[xweb] server closed")
		}
		return
	}
}

//UpdateHandlerTimeout -
func UpdateHandlerTimeout(c *config.AppConfig) {
	if s != nil {
		cc := make(map[string]map[string]time.Duration)
		for path, methods := range c.HandlerTimeout {
			for method, timeout := range methods {
				if timeout == 0 {
					continue
				}
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
}

//StopWebServer -
func StopWebServer() {
	if s != nil {
		s.StopWebServer()
	}
}`

const path = "./server/xweb/"
const name = "xweb.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xweb").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+name, e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
