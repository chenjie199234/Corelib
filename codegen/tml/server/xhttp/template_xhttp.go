package xhttp

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xhttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"{{.}}/api"
	"{{.}}/service"
	"{{.}}/source"

	"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/log"
)

var s *gin.Engine
var hs *http.Server

//StartHttpServer -
func StartHttpServer() {
	s = gin.New()
	c := source.GetHttpConfig()
	if c.HttpCors != nil {
		s.Use(cors.Cors(&cors.Config{
			AllowedOrigin:    c.HttpCors.CorsOrigin,
			AllowedMethod:    c.HttpCors.CorsMethod,
			AllowedHeader:    c.HttpCors.CorsHeader,
			ExposeHeader:     c.HttpCors.CorsExpose,
			AllowCredentials: true,
			MaxAge:           time.Hour * 24,
		}))
	} else {
		s.Use(cors.Cors(nil))
	}
	s.Use(basic.Basic(&basic.Config{
		Timeout: time.Duration(c.HttpTimeout),
	}))
	//you just need to register your service here
	api.RegisterStatusGinServer(s, service.SvcStatus, httpmidware.AllMids())
	//example
	//api.RegisterExampleGinServer(s, service.SvcExample, httpmidware.AllMids())

	hs = &http.Server{
		ReadTimeout:    time.Duration(c.HttpTimeout),
		Handler:        s,
		MaxHeaderBytes: 1024,
	}
	l, e := net.Listen("tcp", fmt.Sprintf(":%d", c.HttpPort))
	if e != nil {
		log.Fatalf("[xhttp]listen http on port:%d error:%s", c.HttpPort, e)
	}
	defer l.Close()
	if c.HttpCertFile != "" && c.HttpKeyFile != "" {
		hs.ServeTLS(l, c.HttpCertFile, c.HttpKeyFile)
	} else {
		hs.Serve(l)
	}
}

//StopHttpServer -
func StopHttpServer() {
	hs.Shutdown(context.Background())
}`

const path = "./server/xhttp/"
const name = "xhttp.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xhttp").Parse(text)
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
