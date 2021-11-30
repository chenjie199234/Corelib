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

	"github.com/chenjie199234/Corelib/grpc"
	"github.com/chenjie199234/Corelib/grpc/mids"
	"github.com/chenjie199234/Corelib/log"
)

var s *grpc.GrpcServer

//StartGrpcServer -
func StartGrpcServer() {
	c := config.GetGrpcServerConfig()
	grpcc := &grpc.ServerConfig{
		GlobalTimeout: time.Duration(c.GlobalTimeout),
		HeartPorbe:    time.Duration(c.HeartProbe),
		SocketRBuf:    2048,
		SocketWBuf:    2048,
		MaxMsgLen:     65535,
	}
	var e error
	if s, e = grpc.NewGrpcServer(grpcc, api.Group, api.Name); e != nil {
		log.Error(nil,"[xgrpc] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusGrpcServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleGrpcServer(s, service.SvcExample, mids.AllMids())

	if e = s.StartGrpcServer(":7000"); e != nil {
		if e != grpc.ErrServerClosed {
			log.Error(nil,"[xgrpc] start error:", e)
		} else {
			log.Info(nil,"[xgrpc] server closed")
		}
		return
	}
}

//UpdateHandlerTimeout -
func UpdateHandlerTimeout(c *config.AppConfig) {
	if s != nil {
		cc := make([]*grpc.HandlerTimeoutConfig, 0, 10)
		for path, methods := range c.HandlerTimeout {
			for method, timeout := range methods {
				method = strings.ToUpper(method)
				if method == "GRPC" {
					cc = append(cc, &grpc.HandlerTimeoutConfig{
						Method:  method,
						Path:    path,
						Timeout: time.Duration(timeout),
					})
				}
			}
		}
		s.UpdateHandlerTimeout(cc)
	}
}

//StopGrpcServer -
func StopGrpcServer() {
	if s != nil {
		s.StopGrpcServer()
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
