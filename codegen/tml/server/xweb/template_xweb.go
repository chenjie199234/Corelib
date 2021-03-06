package xweb

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xweb

import (
	"sync"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/web/mids"
	discoverysdk "github.com/chenjie199234/Discovery/sdk"
)

var s *web.WebServer

//StartWebServer -
func StartWebServer(wg *sync.WaitGroup) {
	c := config.GetWebServerConfig()
	webc := &web.ServerConfig{
		GlobalTimeout:      time.Duration(c.GlobalTimeout),
		IdleTimeout:        time.Duration(c.IdleTimeout),
		HeartProbe:         time.Duration(c.HeartProbe),
		StaticFileRootPath: c.StaticFile,
		MaxHeader:          1024,
		SocketRBuf:         1024,
		SocketWBuf:         1024,
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
		log.Error("[xweb] new error:", e)
		return
	}

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	if e = api.RegisterStatusWebServer(s, service.SvcStatus, mids.AllMids()); e != nil {
		log.Error("[xweb] register handlers error:", e)
		return
	}
	//example
	//if e = api.RegisterExampleWebServer(s, service.SvcExample, mids.AllMids()); e != nil {
	//log.Error("[xweb] register handlers error:", e)
	//return
	//}

	if config.EC.ServerVerifyDatas != nil {
		if e = discoverysdk.RegWeb(8000, "http"); e != nil {
			log.Error("[xweb] register web to discovery server error:", e)
			return
		}
	}
	wg.Done()
	if e = s.StartWebServer(":8000", nil); e != nil {
		log.Error("[xweb] start error:", e)
		return
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
