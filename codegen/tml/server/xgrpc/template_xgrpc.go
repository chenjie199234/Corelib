package xgrpc

import (
	"os"
	"text/template"
)

const txt = `package xgrpc

import (
	"crypto/tls"
	"log/slog"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/cgrpc/mids"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var s *cgrpc.CGrpcServer

// StartCGrpcServer -
func StartCGrpcServer() {
	c := config.GetCGrpcServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				slog.ErrorContext(nil, "[xgrpc] load cert failed:", slog.String("cert", cert), slog.String("key", key), slog.String("error",e.Error()))
				return 
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	var e error
	if s, e = cgrpc.NewCGrpcServer(c.ServerConfig, model.Project, model.Group, model.Name, tlsc); e != nil {
		slog.ErrorContext(nil, "[xgrpc] new server failed", slog.String("error",e.Error()))
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
		slog.ErrorContext(nil, "[xgrpc] start server failed", slog.String("error",e.Error()))
		return
	}
	slog.InfoContext(nil, "[xgrpc] server closed")
}

// UpdateHandlerTimeout -
// first key path,second key method,value timeout duration
func UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	if s!=nil{
		s.UpdateHandlerTimeout(timeout)
	}
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
