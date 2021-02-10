package xgrpc

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package xgrpc

import (
	"fmt"
	"net"

	"{{.}}/api"
	"{{.}}/service"
	"{{.}}/source"

	"github.com/chenjie199234/Corelib/pkg/log"
	"github.com/chenjie199234/Corelib/rpc"
)

var s *grpc.Server

//StartGrpcServer -
func StartGrpcServer() {
	s = grpc.NewServer()
	//you just need to register your service here
	api.RegisterStatusServer(s, service.SvcStatus)
	//example
	//api.RegisterExampleServer(s, service.SvcExample)

	c := source.GetRpcConfig()
	l, e := net.Listen("tcp", fmt.Sprintf(":%d", c.RpcPort))
	if e != nil {
		log.Fatalf("[xgrpc]listen grpc on port:%d error:%s", c.RpcPort, e)
	}
	defer l.Close()
	s.Serve(l)
}

//StopGrpcServer -
func StopGrpcServer() {
	s.GracefulStop()
}`

const path = "./server/xgrpc/"
const name = "xgrpc.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("xgrpc").Parse(text)
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
