package web

import (
	"github.com/chenjie199234/Corelib/internal/picker"
)

type ServerForPick struct {
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}

func (s *ServerForPick) GetServerAddr() string {
	return s.addr
}
