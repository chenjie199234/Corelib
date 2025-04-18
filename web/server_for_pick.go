package web

import (
	"sync/atomic"

	"github.com/chenjie199234/Corelib/internal/picker"
)

type ServerForPick struct {
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server
	closing  atomic.Bool

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}

func (s *ServerForPick) GetServerAddr() string {
	return s.addr
}

func (s *ServerForPick) Pickable() bool {
	return !s.closing.Load()
}
