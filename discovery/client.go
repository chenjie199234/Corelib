package discovery

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"github.com/chenjie199234/Corelib/stream"
)

type client struct {
	nodepool    *sync.Pool
	serveraddrs []string
	verifydata  []byte
	instance    *stream.Instance
}

var clientinstance *client

func (c *client) getnode(peer *stream.Peer, name string, uniqueid uint64) *node {
	result := c.nodepool.Get().(*node)
	result.n = name
	result.p = peer
	result.u = uniqueid
	return result
}
func (c *client) putnode(n *node) {
	n.p = nil
	n.n = ""
	n.u = 0
	c.nodepool.Put(n)
}

func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, serveraddrs []string, vdata []byte) {
	clientinstance = &client{
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &node{}
			},
		},
		serveraddrs: serveraddrs,
		verifydata:  vdata,
	}
	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)
	//clientinstance.instance.StartTcpServer(cc, listenaddr)
	for _, serveraddr := range serveraddrs {
		clientinstance.instance.StartTcpClient(cc, serveraddr, clientinstance.verifydata)
	}
}
func (c *client) verifyfunc(ctx context.Context, peernameip string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
}
func (c *client) userfunc(ctx context.Context, p *stream.Peer, peernameip string, uniqueid uint64, data []byte) {

}
func (c *client) offlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
}
