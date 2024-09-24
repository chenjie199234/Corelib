package xraw

import (
	"os"
	"text/template"
)

const txt = `package xraw

import (
	"context"
	"crypto/tls"
	"log/slog"

	"{{.}}/config"

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
	s, _ = stream.NewInstance(&stream.InstanceConfig{
		TcpC:               &stream.TcpConfig{ConnectTimeout: c.ConnectTimeout.StdDuration()},
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		GroupNum:           c.GroupNum,
		VerifyFunc:         rawVerify,
		OnlineFunc:         rawOnline,
		PingPongFunc:       rawPingPong,
		UserdataFunc:       rawUser,
		OfflineFunc:        rawOffline,
	})

	if e := s.StartServer(":7000", tlsc); e != nil && e != stream.ErrServerClosed {
		slog.ErrorContext(nil, "[xraw] start server failed", slog.String("error", e.Error()))
		return
	}
	slog.InfoContext(nil, "[xraw] server closed")
}

// StopRawServer -
func StopRawServer() {
	if s != nil {
		s.Stop()
	}
}
func rawVerify(ctx context.Context, peerVerifyData []byte) (response []byte, uniqueid string, success bool) {
	return nil, "", false
}
func rawOnline(ctx context.Context, p *stream.Peer) (success bool) {
	return false
}
func rawPingPong(p *stream.Peer) {
}
func rawUser(p *stream.Peer, userdata []byte) {
}
func rawOffline(p *stream.Peer) {
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
