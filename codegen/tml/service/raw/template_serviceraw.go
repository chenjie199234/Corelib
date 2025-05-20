package raw

import (
	"os"
	"text/template"
)

const txt = `package raw

import (
	"context"
	"sync/atomic"
	"unsafe"
	// "log/slog"

	// "{{.}}/config"
	// "{{.}}/api"
	rawdao "{{.}}/dao/raw"
	// "{{.}}/ecode"

	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/graceful"
)

// Service subservice for raw business
type Service struct {
	stop *graceful.Graceful

	rawDao   *rawdao.Dao
	instance *stream.Instance
}

// Start -
func Start() (*Service, error) {
	return &Service{
		stop: graceful.New(),

		//rawDao: rawdao.NewDao(config.GetMysql("raw_mysql"), config.GetRedis("raw_redis"), config.GetMongo("raw_mongo")),
		rawDao: rawdao.NewDao(nil, nil, nil),
	}, nil
}

func (s *Service) SetStreamInstance(instance *stream.Instance) {
	//avoid race when build/run in -race mode
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance)), unsafe.Pointer(instance))
}

func (s *Service) RawVerify(ctx context.Context, peerVerifyData []byte) (response []byte, uniqueid string, success bool) {
	//avoid race when build/run in -race mode
	// instance := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance))))
	return nil, "", false
}

func (s *Service) RawOnline(ctx context.Context, p *stream.Peer) (success bool) {
	//avoid race when build/run in -race mode
	// instance := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance))))
	return false
}

func (s *Service) RawPingPong(p *stream.Peer) {
	//avoid race when build/run in -race mode
	// instance := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance))))
}

func (s *Service) RawUser(p *stream.Peer, userdata []byte) {
	//avoid race when build/run in -race mode
	// instance := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance))))
}

func (s *Service) RawOffline(p *stream.Peer) {
	//avoid race when build/run in -race mode
	// instance := (*stream.Instance)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.instance))))
}

// Stop -
func (s *Service) Stop() {
	s.stop.Close(nil, nil)
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./service/raw/", 0755); e != nil {
		panic("mkdir ./service/raw/ error: " + e.Error())
	}
	servicetemplate, e := template.New("./service/raw/service.go").Parse(txt)
	if e != nil {
		panic("parse ./service/raw/service.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./service/raw/service.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./service/raw/service.go error: " + e.Error())
	}
	if e := servicetemplate.Execute(file, packagename); e != nil {
		panic("write ./service/raw/service.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./service/raw/service.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./service/raw/service.go error: " + e.Error())
	}
}
