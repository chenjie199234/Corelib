package xcrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xcrpc

import (
	"strings"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/crpc/mids"
	"github.com/chenjie199234/Corelib/log"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

var s *crpc.CrpcServer

// StartCrpcServer -
func StartCrpcServer() {
	c := config.GetCrpcServerConfig()
	crpcc := &crpc.ServerConfig{
		ConnectTimeout: time.Duration(c.ConnectTimeout),
		GlobalTimeout:  time.Duration(c.GlobalTimeout),
		HeartPorbe:     time.Duration(c.HeartProbe),
	}
	var e error
	if s, e = crpc.NewCrpcServer(crpcc, model.Group, model.Name); e != nil {
		log.Error(nil,"[xcrpc] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC.HandlerTimeout)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusCrpcServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleCrpcServer(s, service.SvcExample,mids.AllMids())

	if e = s.StartCrpcServer(":9000"); e != nil && e != crpc.ErrServerClosed {
		log.Error(nil,"[xcrpc] start error:", e)
		return
	}
	log.Info(nil,"[xcrpc] server closed")
}

// UpdateHandlerTimeout -
// first key path,second key method,value timeout duration
func UpdateHandlerTimeout(hts map[string]map[string]ctime.Duration) {
	if s == nil {
		return
	}
	cc := make(map[string]time.Duration)
	for path, methods := range hts {
		for method, timeout := range methods {
			method = strings.ToUpper(method)
			if method == "CRPC" {
				cc[path] = timeout.StdDuration()
			}
		}
	}
	s.UpdateHandlerTimeout(cc)
}

// StopCrpcServer -
func StopCrpcServer() {
	if s != nil {
		s.StopCrpcServer()
	}
}`

const path = "./server/xcrpc/"
const name = "xcrpc.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xcrpc").Parse(text)
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
func Execute(PackageName string) {
	if e := tml.Execute(file, PackageName); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
