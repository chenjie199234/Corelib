package cgrpc

import (
	"github.com/chenjie199234/Corelib/internal/picker"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
)

type ServerForPick struct {
	addr     string
	subconn  balancer.SubConn
	dservers map[string]*struct{} //this app registered on which discovery server
	status   int32
	closing  bool

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}
func (s *ServerForPick) GetServerAddr() string {
	return s.addr
}
func (s *ServerForPick) Pickable() bool {
	return s.status == int32(connectivity.Ready) && !s.closing
}
