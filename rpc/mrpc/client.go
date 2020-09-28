package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRCINIT = fmt.Errorf("[Mrpc.client]not init,call NewMrpcClient first")
	ERRCADD  = fmt.Errorf("[Mrpc.client]already exist")
)

//appuniquename = appname:addr
type Client struct {
	appname    string
	verifydata []byte
	instance   *stream.Instance
	callid     uint64
	lker       *sync.RWMutex
	servers    map[string]*serverinfo //key appuniquename
	serverpool *sync.Pool
	reqpool    *sync.Pool
	offlinech  chan string
	noticech   chan *discovery.NoticeMsg
	cc         *stream.TcpConfig
}

type serverinfo struct {
	lker          *sync.Mutex
	appuniquename string
	//key discoveryserver uniquename
	discoveryserver map[string]struct{} //this app registered on which discovery server
	addition        []byte              //addition info registered on discovery server
	cpu             float64             //cpu use percent
	mem             float64             //men use percent
	netlag          []int64             //the net lag collect samples
	netlagindex     int                 //the samples index
	peer            *stream.Peer
	uniqueid        uint64
	status          int //0-idle,1-start,2-verify,3-connected
}

func NewMrpcClient(c *stream.InstanceConfig, cc *stream.TcpConfig, appname string, vdata []byte) *Client {
	clientinstance := &Client{
		appname:    appname,
		verifydata: vdata,
		callid:     0,
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*serverinfo, 10),
		serverpool: &sync.Pool{},
		offlinech:  make(chan string, 5),
		cc:         cc,
	}
	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)
	odata, noticech, e := discovery.TcpNotice(appname)
	if e != nil {
		fmt.Printf("[Mrpc.client.NewMrpcClient.impossible]add app:%s notice for register info error:%s\n", appname, e)
		return nil
	}
	clientinstance.noticech = noticech
	clientinstance.first(odata)
	go clientinstance.notice()
	return clientinstance
}
func (c *Client) first(data map[string]map[string][]byte) {
	for addr, discoveryservers := range data {
		s := c.getserver()
		s.appuniquename = fmt.Sprintf("%s:%s", c.appname, addr)
		tempdiscoveryservers := make(map[string]struct{}, len(discoveryservers))
		tempaddition := []byte{}
		for addr, addition := range discoveryservers {
			if len(tempaddition) == 0 {
				tempaddition = addition
			} else if !bytes.Equal(tempaddition, addition) {
				fmt.Printf("[Mrpc.client.NewMrpcClient.impossible]peer:%s addition info conflict\n", s.appuniquename)
				return
			}
			tempdiscoveryservers[addr] = struct{}{}
		}
		s.discoveryserver = tempdiscoveryservers
		s.addition = tempaddition
		s.status = 1
		c.lker.Lock()
		c.servers[s.appuniquename] = s
		c.lker.Unlock()
		go c.start(addr)
	}
}
func (c *Client) notice() {
	for {
		data := <-c.noticech
		appuniquename := fmt.Sprintf("%s:%s", c.appname, data.PeerAddr)
		c.lker.RLock()
		server, ok := c.servers[appuniquename]
		c.lker.RUnlock()
		if ok && data.Status {
			//this peer exist,it register on another discovery server
			server.lker.Lock()
			if _, ok := server.discoveryserver[data.DiscoveryServer]; ok {
				//already registered on this discovery server
				//this is impossible
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s duplicate register on discoveryserver:%s\n", appuniquename, data.DiscoveryServer)
			} else if !bytes.Equal(server.addition, data.Addition) {
				//register with different registerinfo
				//this is impossible
				server.discoveryserver[data.DiscoveryServer] = struct{}{}
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s addition info conflict\n", appuniquename)
			} else {
				server.discoveryserver[data.DiscoveryServer] = struct{}{}
			}
			server.lker.Unlock()
		} else if ok {
			//this peer exist,it unregister on a discovery server
			server.lker.Lock()
			if _, ok := server.discoveryserver[data.DiscoveryServer]; !ok {
				//didn't registered on this discovery server before
				//this is impossible
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s duplicate unregister on discoveryserver:%s\n", appuniquename, data.DiscoveryServer)
			} else if !bytes.Equal(server.addition, data.Addition) {
				//this is impossible
				delete(server.discoveryserver, data.DiscoveryServer)
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s addition info conflict\n", appuniquename)
			} else {
				delete(server.discoveryserver, data.DiscoveryServer)
			}
			needoffline := false
			if len(server.discoveryserver) == 0 && server.status == 0 {
				needoffline = true
			} else if len(server.discoveryserver) != 0 && server.status == 0 {
				server.status = 1
				go c.start(data.PeerAddr)
			}
			server.lker.Unlock()
			if needoffline {
				c.unregister(appuniquename)
			}
		} else if data.Status {
			//this peer not exist,it register on a discovery server
			server = c.getserver()
			server.appuniquename = appuniquename
			server.discoveryserver[data.DiscoveryServer] = struct{}{}
			server.addition = data.Addition
			server.status = 1
			c.lker.Lock()
			c.servers[appuniquename] = server
			c.lker.Unlock()
			go c.start(data.PeerAddr)
		} else {
			//this peer not exist,it unregister on a discovery server
			//this is impossible
			fmt.Printf("[Mprc.client.notice.impossible]app:%s duplicate unregister on discoveryserver:%s\n", appuniquename, data.DiscoveryServer)
			return
		}
	}
}
func (c *Client) unregister(appuniquename string) {
	c.lker.Lock()
	server, ok := c.servers[appuniquename]
	if !ok {
		//this is impossible
		c.lker.Unlock()
		return
	}
	//check again
	server.lker.Lock()
	if len(server.discoveryserver) == 0 && server.status == 0 {
		delete(c.servers, appuniquename)
		c.putserver(server)
	} else if len(server.discoveryserver) != 0 && server.status == 0 {
		server.status = 1
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
	server.lker.Unlock()
	c.lker.Unlock()
}
func (c *Client) start(addr string) {
	if r := c.instance.StartTcpClient(c.cc, addr, c.verifydata); r == "" {
		appuniquename := fmt.Sprintf("%s:%s", c.appname, addr)
		c.lker.RLock()
		server, ok := c.servers[appuniquename]
		if !ok {
			c.lker.RUnlock()
			return
		}
		server.lker.Lock()
		c.lker.RUnlock()
		needoffline := len(server.discoveryserver) == 0
		if needoffline {
			server.status = 0
		} else {
			server.status = 1
			go c.start(addr)
		}
		server.lker.Unlock()
		if needoffline {
			c.unregister(appuniquename)
		}
	}
}
func (s *serverinfo) reset() {
	s.appuniquename = ""
	s.discoveryserver = make(map[string]struct{}, 2)
	s.addition = nil
	s.cpu = 0
	s.mem = 0
	for i := 0; i < 30; i++ {
		s.netlag[i] = 0
	}
	s.netlagindex = 0
	s.peer = nil
	s.uniqueid = 0
	s.status = 0
}
func (c *Client) getserver() *serverinfo {
	s, ok := c.serverpool.Get().(*serverinfo)
	if ok {
		s.reset()
		return s
	}
	return &serverinfo{
		lker:            &sync.Mutex{},
		appuniquename:   "",
		discoveryserver: make(map[string]struct{}, 2),
		addition:        nil,
		cpu:             0,
		mem:             0,
		netlag:          make([]int64, 30),
		netlagindex:     0,
		peer:            nil,
		uniqueid:        0,
		status:          0,
	}
}
func (c *Client) putserver(s *serverinfo) {
	s.reset()
	c.serverpool.Put(s)
}
func (c *Client) verifyfunc(ctx context.Context, appuniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	c.lker.RLock()
	server, ok := c.servers[appuniquename]
	if !ok || server.peer != nil || server.uniqueid != 0 {
		c.lker.RUnlock()
		return nil, false
	}
	server.lker.Lock()
	c.lker.RUnlock()
	if server.status != 1 {
		server.lker.Unlock()
		return nil, false
	}
	server.status = 2
	server.lker.Unlock()
	return nil, true
}
func (c *Client) onlinefunc(p *stream.Peer, appuniquename string, uniqueid uint64) {
	c.lker.RLock()
	server, ok := c.servers[appuniquename]
	if !ok {
		c.lker.RUnlock()
		return
	}
	server.lker.Lock()
	c.lker.RUnlock()
	if server.status == 2 {
		server.peer = p
		server.uniqueid = uniqueid
		server.status = 3
		p.SetData(unsafe.Pointer(server), uniqueid)
	} else {
		//this is impossible
		p.Close(uniqueid)
	}
	server.lker.Unlock()
}
func (c *Client) userfunc(p *stream.Peer, appuniquename string, uniqueid uint64, data []byte) {
}
func (c *Client) offlinefunc(p *stream.Peer, appuniquename string, uniqueid uint64) {
	tempserver, e := p.GetData(uniqueid)
	if e != nil {
		//this is impossible
		fmt.Printf("[Mrpc.client.offlinefunc.impossible]server offline before offlinefunc called\n")
		return
	}
	server := (*serverinfo)(tempserver)
	server.lker.Lock()
	server.peer = nil
	server.uniqueid = 0
	needoffline := len(server.discoveryserver) == 0
	if needoffline {
		server.status = 0
	} else {
		server.status = 1
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
	server.lker.Unlock()
	if needoffline {
		c.unregister(appuniquename)
	}
}

type reqinfo struct {
}

func (c *Client) Call() {

}
func (r *reqinfo) reset() {

}
func (c *Client) getreq() *reqinfo {
	r, ok := c.reqpool.Get().(*reqinfo)
	if ok {
		r.reset()
		return r
	}
	return &reqinfo{}
}
func (c *Client) putreq(r *reqinfo) {
	r.reset()
	c.reqpool.Put(r)
}
