package xcrpc

import (
	"os"
	"text/template"
)

const txt = `package xcrpc

import (
	"crypto/tls"
	"log/slog"
	"sync/atomic"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/crpc/mids"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var s atomic.Pointer[crpc.CrpcServer]

func StartCrpcServer() {
	c := config.GetCrpcServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				slog.Error("[xcrpc] load cert failed:", slog.String("cert", cert), slog.String("key", key), slog.String("error",e.Error()))
				return 
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	server, e := crpc.NewCrpcServer(c.ServerConfig, tlsc)
	if e != nil {
		slog.Error("[xcrpc] new server failed", slog.String("error",e.Error()))
		return
	}
	s.Store(server)
	UpdateHandlerTimeout(config.AC.HandlerTimeout)

	//this place can register global midwares
	//server.Use(globalmidwares)

	//example
	//api.RegisterExampleCrpcServer(server, service.SvcExample,mids.AllMids())
	//you need to register your service here
	api.RegisterStatusCrpcServer(server, service.SvcStatus, mids.AllMids())

	if e = server.StartCrpcServer(":9000"); e != nil && e != crpc.ErrServerClosed {
		slog.Error("[xcrpc] start server failed", slog.String("error",e.Error()))
		return
	}
	slog.Info("[xcrpc] server closed")
}

//first key:path,second key:method
func UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmps := s.Load()
	if tmps != nil {
		tmps.UpdateHandlerTimeout(timeout)
	}
}

func StopCrpcServer(force bool) {
	tmps := s.Load()
	if tmps != nil {
		tmps.StopCrpcServer(force)
	}
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./server/xcrpc/", 0755); e != nil {
		panic("mkdir ./server/xcrpc/ error: " + e.Error())
	}
	xcrpctemplate, e := template.New("./server/xcrpc/xcrpc.go").Parse(txt)
	if e != nil {
		panic("parse ./server/xcrpc/xcrpc.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./server/xcrpc/xcrpc.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./server/xcrpc/xcrpc.go error: " + e.Error())
	}
	if e := xcrpctemplate.Execute(file, packagename); e != nil {
		panic("write ./server/xcrpc/xcrpc.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./server/xcrpc/xcrpc.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./server/xcrpc/xcrpc.go error: " + e.Error())
	}
}
