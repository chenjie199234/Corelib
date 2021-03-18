package xweb

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xweb

import (
	"os"
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
	c := config.GetWebConfig()
	newvd := os.Getenv("PPROF_VERIFY_DATA")
	if c.UsePprof && (newvd == "<PPROF_VERIFY_DATA>" || newvd == "") {
		log.Error("[xweb] missing verifydata")
		return
	}
	oldvd := os.Getenv("OLD_PPROF_VERIFY_DATA")
	if oldvd == "<OLD_PPROF_VERIFY_DATA>" {
		oldvd = ""
	}
	webc := &web.Config{
		UsePprof:           c.UsePprof,
		Timeout:            time.Duration(c.WebTimeout),
		StaticFileRootPath: c.WebStaticFile,
		MaxHeader:          1024,
		ReadBuffer:         1024,
		WriteBuffer:        1024,
		PprofVerifyData:    newvd,
		OldPprofVerifyData: oldvd,
	}
	if c.WebCors != nil {
		webc.Cors = &web.CorsConfig{
			AllowedOrigin:    c.WebCors.CorsOrigin,
			AllowedHeader:    c.WebCors.CorsHeader,
			ExposeHeader:     c.WebCors.CorsExpose,
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

	if e = s.StartWebServer(":8000", c.WebCertFile, c.WebKeyFile); e != nil {
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
		panic(fmt.Sprintf("create template for %s error:%s", path+name, e))
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
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+name, e))
	}
}
