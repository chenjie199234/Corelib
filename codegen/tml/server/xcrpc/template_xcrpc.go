package xcrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xcrpc

import (
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/crpc/mids"
	"github.com/chenjie199234/Corelib/log"
)

var s *crpc.CrpcServer

//StartCrpcServer -
func StartCrpcServer() {
	c := config.GetCrpcServerConfig()
	crpcc := &crpc.ServerConfig{
		GlobalTimeout: time.Duration(c.GlobalTimeout),
		HeartPorbe:    time.Duration(c.HeartProbe),
		GroupNum:      1,
		SocketRBuf:    2048,
		SocketWBuf:    2048,
		MaxMsgLen:     65535,
		VerifyDatas:   config.EC.ServerVerifyDatas,
	}
	var e error
	if s, e = crpc.NewCrpcServer(crpcc, api.Group, api.Name); e != nil {
		log.Error(nil,"[xcrpc] new error:", e)
		return
	}

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	if e = api.RegisterStatusCrpcServer(s, service.SvcStatus, mids.AllMids()); e != nil {
		log.Error(nil,"[xcrpc] register handlers error:", e)
		return
	}
	//example
	//if e = api.RegisterExampleCrpcServer(s, service.SvcExample,mids.AllMids()); e != nil {
	//log.Error(nil,"[xcrpc] register handlers error:", e)
	//return
	//}

	if e = s.StartCrpcServer(":9000", nil); e != nil {
		if e != crpc.ErrServerClosed {
			log.Error(nil,"[xcrpc] start error:", e)
		} else {
			log.Info(nil,"[xcrpc] server closed")
		}
		return
	}
}

//StopCrpcServer -
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
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}