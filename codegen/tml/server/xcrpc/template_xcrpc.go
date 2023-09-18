package xcrpc

import (
	"os"
	"text/template"
)

const txt = `package xcrpc

import (
	"crypto/tls"

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/model"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/crpc/mids"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var s *crpc.CrpcServer

// StartCrpcServer -
func StartCrpcServer() {
	c := config.GetCrpcServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				log.Error(nil, "[xcrpc] load cert failed:",map[string]interface{}{"cert": cert, "key": key, "error": e})
				return 
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	var e error
	if s, e = crpc.NewCrpcServer(c.ServerConfig, model.Project, model.Group, model.Name, tlsc); e != nil {
		log.Error(nil, "[xcrpc] new server failed", map[string]interface{}{"error": e})
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
		log.Error(nil, "[xcrpc] start server failed", map[string]interface{}{"error": e})
		return
	}
	log.Info(nil, "[xcrpc] server closed", nil)
}

// UpdateHandlerTimeout -
// first key path,second key method,value timeout duration
func UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	if s != nil {
		s.UpdateHandlerTimeout(timeout)
	}
}

// StopCrpcServer force - false(graceful),true(not graceful)
func StopCrpcServer(force bool) {
	if s != nil {
		s.StopCrpcServer(force)
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
