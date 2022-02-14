package xgrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xgrpc

import (
	"strings"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/cgrpc/mids"
	"github.com/chenjie199234/Corelib/log"
)

var s *cgrpc.CGrpcServer

//StartCGrpcServer -
func StartCGrpcServer() {
	c := config.GetCGrpcServerConfig()
	cgrpcc := &cgrpc.ServerConfig{
		ConnectTimeout: time.Duration(c.ConnectTimeout),
		GlobalTimeout:  time.Duration(c.GlobalTimeout),
		HeartPorbe:     time.Duration(c.HeartProbe),
	}
	var e error
	if s, e = cgrpc.NewCGrpcServer(cgrpcc, api.Group, api.Name); e != nil {
		log.Error(nil,"[xgrpc] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusCGrpcServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleCGrpcServer(s, service.SvcExample, mids.AllMids())

	if e = s.StartCGrpcServer(":10000"); e != nil && e != cgrpc.ErrServerClosed {
		log.Error(nil,"[xgrpc] start error:", e)
		return
	}
	log.Info(nil,"[xgrpc] server closed")
}

//UpdateHandlerTimeout -
func UpdateHandlerTimeout(c *config.AppConfig) {
	if s != nil {
		cc := make(map[string]time.Duration)
		for path, methods := range c.HandlerTimeout {
			for method, timeout := range methods {
				if timeout == 0 {
					continue
				}
				method = strings.ToUpper(method)
				if method == "GRPC" {
					cc[path] = timeout.StdDuration()
				}
			}
		}
		s.UpdateHandlerTimeout(cc)
	}
}

//StopCGrpcServer -
func StopCGrpcServer() {
	if s != nil {
		s.StopCGrpcServer()
	}
}`

const path = "./server/xgrpc/"
const name = "xgrpc.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xgrpc").Parse(text)
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
