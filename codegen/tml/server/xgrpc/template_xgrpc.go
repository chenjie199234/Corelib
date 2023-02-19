package xgrpc

import (
	"os"
	"text/template"
)

const txt = `package xgrpc

import (
	"strings"
	"time"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/cgrpc/mids"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var s *cgrpc.CGrpcServer

// StartCGrpcServer -
func StartCGrpcServer() {
	c := config.GetCGrpcServerConfig()
	cgrpcc := &cgrpc.ServerConfig{
		ConnectTimeout: time.Duration(c.ConnectTimeout),
		GlobalTimeout:  time.Duration(c.GlobalTimeout),
		HeartPorbe:     time.Duration(c.HeartProbe),
		Certs:          c.Certs,
	}
	var e error
	if s, e = cgrpc.NewCGrpcServer(cgrpcc, model.Group, model.Name); e != nil {
		log.Error(nil, "[xgrpc] new error:", e)
		return
	}
	UpdateHandlerTimeout(config.AC.HandlerTimeout)

	//this place can register global midwares
	//s.Use(globalmidwares)

	//you just need to register your service here
	api.RegisterStatusCGrpcServer(s, service.SvcStatus, mids.AllMids())
	//example
	//api.RegisterExampleCGrpcServer(s, service.SvcExample, mids.AllMids())

	if e = s.StartCGrpcServer(":10000"); e != nil && e != cgrpc.ErrServerClosed {
		log.Error(nil, "[xgrpc] start error:", e)
		return
	}
	log.Info(nil, "[xgrpc] server closed")
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
			if method == "GRPC" {
				cc[path] = timeout.StdDuration()
			}
		}
	}
	s.UpdateHandlerTimeout(cc)
}

// StopCGrpcServer force - false(graceful),true(not graceful)
func StopCGrpcServer(force bool) {
	if s != nil {
		s.StopCGrpcServer(force)
	}
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./server/xgrpc/", 0755); e != nil {
		panic("mkdir ./server/xgrpc/ error: " + e.Error())
	}
	xgrpctemplate, e := template.New("./server/xgrpc/xgrpc.go").Parse(txt)
	if e != nil {
		panic("parse ./server/xgrpc/xgrpc.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./server/xgrpc/xgrpc.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./server/xgrpc/xgrpc.go error: " + e.Error())
	}
	if e := xgrpctemplate.Execute(file, packagename); e != nil {
		panic("write ./server/xgrpc/xgrpc.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./server/xgrpc/xgrpc.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./server/xgrpc/xgrpc.go error: " + e.Error())
	}
}
