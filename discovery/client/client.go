package client

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"github.com/chenjie199234/Corelib/stream"
)

var verifydata []byte

//tcp instance
var instance map[string]*stream.Instance

func NewDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, serveraddrs []string, vdata []byte) {
	c.Verifyfunc = verifyfunc
	c.Onlinefunc = onlinefunc
	c.Userdatafunc = userfunc
	c.Offlinefunc = offlinefunc
}
func verifyfunc(ctx context.Context, peername string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, verifydata) {
		return nil, false
	}
	return nil, true
}
func onlinefunc(p *stream.Peer, peername string, uniqueid uint64) {
}
func userfunc(ctx context.Context, p *stream.Peer, peername string, uniqueid uint64, data []byte) {

}
func offlinefunc(p *stream.Peer, peername string, uniqueid uint64) {
}
func splitNameIp(nameip string) (string, string) {
	splits := strings.Split(nameip, ",")
	return splits[0], splits[1]
}
