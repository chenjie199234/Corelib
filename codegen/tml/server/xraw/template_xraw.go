package xraw

import (
	"os"
	"text/template"
)

const txt = `package xraw

import (
	"crypto/tls"
	"log/slog"
	"sync/atomic"
	"unsafe"

	"{{.}}/config"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/stream"
)

var s *stream.Instance

// StartRawServer -
func StartRawServer() {
	c := config.GetRawServerConfig()
	var tlsc *tls.Config
	if len(c.Certs) > 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				slog.ErrorContext(nil, "[xraw] load cert failed:", slog.String("cert", cert), slog.String("key", key), slog.String("error", e.Error()))
				return
			}
			certificates = append(certificates, temp)
		}
		tlsc = &tls.Config{Certificates: certificates}
	}
	server, _ := stream.NewInstance(&stream.InstanceConfig{
		TcpC:               &stream.TcpConfig{ConnectTimeout: c.ConnectTimeout.StdDuration(), MaxMsgLen: c.MaxMsgLen},
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		GroupNum:           c.GroupNum,
		VerifyFunc:         service.SvcRaw.RawVerify,
		OnlineFunc:         service.SvcRaw.RawOnline,
		PingPongFunc:       service.SvcRaw.RawPingPong,
		UserdataFunc:       service.SvcRaw.RawUser,
		OfflineFunc:        service.SvcRaw.RawOffline,
	})
	//avoid race when build/run in -race mode
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s)), unsafe.Pointer(server))

	service.SvcRaw.SetStreamInstance(server)

	if e := server.StartServer(":7000", tlsc); e != nil && e != stream.ErrServerClosed {
		slog.ErrorContext(nil, "[xraw] start server failed", slog.String("error", e.Error()))
		return
	}
	slog.InfoContext(nil, "[xraw] server closed")
}

// StopRawServer -
func StopRawServer() {
	//avoid race when build/run in -race mode
	tmps := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s))))
	if tmps != nil {
		tmps.Stop()
	}
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./server/xraw/", 0755); e != nil {
		panic("mkdir ./server/xraw/ error: " + e.Error())
	}
	xcrpctemplate, e := template.New("./server/xraw/xraw.go").Parse(txt)
	if e != nil {
		panic("parse ./server/xraw/xraw.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./server/xraw/xraw.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./server/xraw/xraw.go error: " + e.Error())
	}
	if e := xcrpctemplate.Execute(file, packagename); e != nil {
		panic("write ./server/xraw/xraw.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./server/xraw/xraw.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./server/xraw/xraw.go error: " + e.Error())
	}
}
