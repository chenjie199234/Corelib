package xrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xrpc

import (
	"fmt"
	"time"

	"{{.}}/api"
	"{{.}}/service"
	"{{.}}/source"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/rpc"
	"github.com/chenjie199234/Corelib/rpc/mids"
	"github.com/chenjie199234/Corelib/stream"
)

var s *rpc.RpcServer

//StartRpcServer -
func StartRpcServer() {
	c := source.GetRpcConfig()
	rpcc := &stream.InstanceConfig{
		SelfName:           "{{.}}",
		HeartbeatTimeout:   time.Duration(c.RpcHeartTimeout),
		HeartprobeInterval: time.Duration(c.RpcHeartProbe),
		TcpC: &stream.TcpConfig{
			ConnectTimeout:    time.Duration(c.RpcConnTimeout),
			AppWriteBufferNum: 65535,
		},
	}
	var e error
	if s, e = rpc.NewRpcServer(rpcc, time.Duration(c.RpcTimeout), []byte(c.RpcVerifydata)); e != nil {
		log.Error("[xrpc] new rpc server error:", e)
		return
	}

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	if e := api.RegisterStatusRpcServer(s, service.SvcStatus, mids.AllMids()); e != nil {
		log.Error("[xrpc] register handlers error:", e)
		return
	}
	//example
	//if e := api.RegisterExampleRpcServer(s, service.SvcExample,mids.AllMids()); e != nil {
	//log.Error("[xrpc] register handlers error:", e)
	//return
	//}

	if e := s.StartRpcServer(fmt.Sprintf(":%d", c.RpcPort)); e != nil {
		log.Error("[xrpc] start rpc server error:", e)
		return
	}
}

//StopRpcServer -
func StopRpcServer() {
	s.StopRpcServer()
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
