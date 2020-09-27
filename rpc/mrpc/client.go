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
	serverpool *sync.Pool
	reqpool    *sync.Pool
}

type serverinfo struct {
	appuniquename string
	//key discoveryserver uniquename
	discoveryserver map[string][]byte //this app registered on which discovery server
	cpu             float64           //cpu use percent
	mem             float64           //men use percent
	netlag          []int64           //the net lag collect samples
	netlagindex     int               //the
	peer            *stream.Peer
	uniqueid        uint64
}
type reqinfo struct {
}

func NewMrpcClient(c *stream.InstanceConfig, cc *stream.TcpConfig, appname string, vdata []byte) *client {
	clientinstance := &client{
		verifydata: vdata,
		callid:     0,
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*serverinfo, 10),
		serverpool: &sync.Pool{},
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
		clientinstance.odata(odata, cc)
	}
	go func() {
		for {
			ndata := <-notice
			clientinstance.ndata(ndata, cc)
		}
	}()
	return clientinstance
}
func (s *serverinfo) reset() {
	s.discoveryserver = make(map[string][]byte)
	s.cpu = 0
	s.mem = 0
	for i := 0; i < 30; i++ {
		s.netlag[i] = 0
	}
	s.netlagindex = 0
	s.peer = nil
	s.uniqueid = 0
}
func (c *client) getserver() *serverinfo {
	s, ok := c.serverpool.Get().(*serverinfo)
	if ok {
		s.reset()
		return s
	}
	return &serverinfo{
		discoveryserver: make(map[string][]byte),
		cpu:             0,
		mem:             0,
		netlag:          make([]int64, 30),
		netlagindex:     0,
		peer:            nil,
		uniqueid:        0,
	}
}
func (c *client) putserver(s *serverinfo) {
	s.reset()
	c.serverpool.Put(s)
}
func (r *reqinfo) reset() {

}
func (c *client) getreq() *reqinfo {
	r, ok := c.reqpool.Get().(*reqinfo)
	if ok {
		r.reset()
		return r
	}
	return &reqinfo{}
}
func (c *client) putreq(r *reqinfo) {
	r.reset()
	c.reqpool.Put(r)
}
func (c *client) odata(data map[string]map[string][]byte, cc *stream.TcpConfig) {
	for addr, discoveryservers := range data {
		c.instance.StartTcpClient(cc, addr, c.verifydata)
	}
}
func (c *client) ndata(data *discovery.NoticeMsg, cc *stream.TcpConfig) {

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
