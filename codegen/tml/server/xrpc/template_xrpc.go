package xrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xrpc

import (
	"sync"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/rpc"
	"github.com/chenjie199234/Corelib/rpc/mids"
	discoverysdk "github.com/chenjie199234/Discovery/sdk"
)

var s *rpc.RpcServer

//StartRpcServer -
func StartRpcServer(wg *sync.WaitGroup) {
	c := config.GetRpcServerConfig()
	rpcc := &rpc.ServerConfig{
		GlobalTimeout:          time.Duration(c.GlobalTimeout),
		HeartTimeout:           time.Duration(c.HeartTimeout),
		HeartPorbe:             time.Duration(c.HeartProbe),
		GroupNum:               1,
		SocketRBuf:             1024,
		SocketWBuf:             1024,
		MaxMsgLen:              65535,
		MaxBufferedWriteMsgNum: 1024,
		VerifyDatas:            config.EC.ServerVerifyDatas,
	}
	var e error
	if s, e = rpc.NewRpcServer(rpcc, api.Group, api.Name); e != nil {
		log.Error("[xrpc] new error:", e)
		return
	}

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	if e = api.RegisterStatusRpcServer(s, service.SvcStatus, mids.AllMids()); e != nil {
		log.Error("[xrpc] register handlers error:", e)
		return
	}
	//example
	//if e = api.RegisterExampleRpcServer(s, service.SvcExample,mids.AllMids()); e != nil {
	//log.Error("[xrpc] register handlers error:", e)
	//return
	//}

	if config.EC.ServerVerifyDatas != nil {
		if e = discoverysdk.RegRpc(9000); e != nil {
			log.Error("[xrpc] register rpc to discovery server error:", e)
			return
		}
	}
	wg.Done()
	if e = s.StartRpcServer(":9000"); e != nil {
		log.Error("[xrpc] start error:", e)
		return
	}
}

//StopRpcServer -
func StopRpcServer() {
	if s != nil {
		s.StopRpcServer()
	}
}`

const path = "./server/xrpc/"
const name = "xrpc.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xrpc").Parse(text)
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
