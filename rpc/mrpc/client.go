package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRCINIT = fmt.Errorf("[Mrpc.client]not init,call NewMrpcClient first")
	ERRCADD  = fmt.Errorf("[Mrpc.client]already exist")
)

//appuniquename = appname:addr
type client struct {
	verifydata []byte
	instance   *stream.Instance
	callid     uint64
	lker       *sync.RWMutex
	servers    map[string]*serverinfo //key appuniquename
	reqpool    *sync.Pool
}

type serverinfo struct {
	discoveryserver map[string]struct{} //this app registered on which discovery server
	cpu             float64             //cpu use percent
	mem             float64             //men use percent
	netlag          []int64             //the lastest net lag
	netlaghead      int
	netlagtail      int
	peer            *stream.Peer
	uniqueid        uint64
}

func NewMrpcClient(c *stream.InstanceConfig, cc *stream.TcpConfig, appname string, vdata []byte) *client {
	clientinstance := &client{
		verifydata: vdata,
		callid:     0,
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*serverinfo, 10),
	}
	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)
	odata, notice, e := discovery.TcpNotice(appname)
	if e != nil {
		return nil
	}
	if len(odata) > 0 {
		clientinstance.odata(odata)
	}
	go func() {
		for {
			ndata := <-notice
			clientinstance.ndata(ndata)
		}
	}()
	return clientinstance
}
func (c *client) odata(data map[string]map[string][]byte) {

}
func (c *client) ndata(data *discovery.NoticeMsg) {

}
func (c *client) verifyfunc(ctx context.Context, appuniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, appuniquename string, uniqueid uint64) {
}
func (c *client) userfunc(p *stream.Peer, appuniquename string, uniqueid uint64, data []byte) {
}
func (c *client) offlinefunc(p *stream.Peer, appuniquename string, uniqueid uint64) {
}
