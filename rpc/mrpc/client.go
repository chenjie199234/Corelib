package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/sys/trace"

	"google.golang.org/protobuf/proto"
)

var (
	ERRCINIT = fmt.Errorf("[Mrpc.client]not init,call NewMrpcClient first")
	ERRCADD  = fmt.Errorf("[Mrpc.client]already exist")
)

var lker *sync.Mutex

//key appname
var clients map[string]*Client

func init() {
	lker = &sync.Mutex{}
	clients = make(map[string]*Client)
	rand.Seed(time.Now().UnixNano())
}

//appuniquename = appname:addr
type Client struct {
	c          *stream.InstanceConfig
	appname    string
	verifydata []byte
	instance   *stream.Instance

	lker       *sync.RWMutex
	servers    map[string]*Serverinfo //key appuniquename
	serverpool *sync.Pool
	cc         *stream.TcpConfig
	noticech   chan *discovery.NoticeMsg
	offlinech  chan string

	callid  uint64
	reqpool *sync.Pool
	pick    func(map[*Serverinfo]PickInfo) *Serverinfo
}

type Serverinfo struct {
	lker          *sync.Mutex
	appuniquename string
	//key discoveryserver uniquename
	discoveryserver map[string]struct{} //this app registered on which discovery server
	peer            *stream.Peer
	starttime       uint64
	status          int //0-idle,1-start,2-verify,3-connected,4-closing

	//active calls
	reqs map[uint64]*reqinfo //all reqs to this server

	pickinfo *PickInfo
}
type PickInfo struct {
	Cpu                        float64 //cpuinfo
	Netlag                     int64   //netlaginfo
	Activecalls                int     //current active calls
	DiscoveryServers           int     //this server registered on how many discoveryservers
	DiscoveryServerOfflineTime int64   //
	Addition                   []byte  //addition info register on register center
}

func (s *Serverinfo) reset() {
	s.appuniquename = ""
	s.discoveryserver = make(map[string]struct{}, 2)
	s.peer = nil
	s.starttime = 0
	s.status = 0

	s.reqs = make(map[uint64]*reqinfo, 10)

	s.pickinfo.Cpu = 0
	s.pickinfo.Netlag = 0
	s.pickinfo.Activecalls = 0
	s.pickinfo.DiscoveryServers = 0
	s.pickinfo.Addition = nil
}
func (c *Client) getserver(appuniquename string, discoveryservers map[string]struct{}, addition []byte) *Serverinfo {
	s, ok := c.serverpool.Get().(*Serverinfo)
	if ok {
		s.reset()
		s.appuniquename = appuniquename
		s.discoveryserver = discoveryservers
		s.pickinfo.Addition = addition
		s.status = 1
		return s
	}
	return &Serverinfo{
		lker:            &sync.Mutex{},
		appuniquename:   appuniquename,
		discoveryserver: discoveryservers,
		peer:            nil,
		starttime:       0,
		status:          1,

		reqs: make(map[uint64]*reqinfo, 10),

		pickinfo: &PickInfo{
			Cpu:              0,
			Netlag:           0,
			Activecalls:      0,
			DiscoveryServers: len(discoveryservers),
			Addition:         addition,
		},
	}
}
func (c *Client) putserver(s *Serverinfo) {
	s.reset()
	c.serverpool.Put(s)
}

func NewMrpcClient(c *stream.InstanceConfig, cc *stream.TcpConfig, appname string, vdata []byte, pick func(map[*Serverinfo]PickInfo) *Serverinfo) *Client {
	//prevent duplicate create
	lker.Lock()
	if c, ok := clients[appname]; ok {
		lker.Unlock()
		return c
	}
	client := &Client{
		c:          c,
		appname:    appname,
		verifydata: vdata,
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*Serverinfo, 10),
		serverpool: &sync.Pool{},
		offlinech:  make(chan string, 5),
		cc:         cc,
		callid:     0,
		reqpool:    &sync.Pool{},
		pick:       pick,
	}
	//tcp instalce
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = client.verifyfunc
	dupc.Onlinefunc = client.onlinefunc
	dupc.Userdatafunc = client.userfunc
	dupc.Offlinefunc = client.offlinefunc
	client.instance = stream.NewInstance(&dupc)
	odata, noticech, e := discovery.TcpNotice(appname)
	if e != nil {
		fmt.Printf("[Mrpc.client.NewMrpcClient.impossible]add app:%s notice for register info error:%s\n", appname, e)
		return nil
	}
	client.noticech = noticech
	client.first(odata)
	go client.notice()
	clients[appname] = client
	lker.Unlock()
	return client
}
func (c *Client) first(data map[string]map[string][]byte) {
	for addr, discoveryservers := range data {
		appuniquename := fmt.Sprintf("%s:%s", c.appname, addr)
		tempdiscoveryservers := make(map[string]struct{}, len(discoveryservers))
		tempaddition := []byte{}
		for discoveryserver, addition := range discoveryservers {
			if len(tempaddition) == 0 {
				tempaddition = addition
			} else if !bytes.Equal(tempaddition, addition) {
				fmt.Printf("[Mrpc.client.NewMrpcClient.impossible]peer:%s addition info conflict\n", appuniquename)
				return
			}
			tempdiscoveryservers[discoveryserver] = struct{}{}
		}
		c.lker.Lock()
		c.servers[appuniquename] = c.getserver(appuniquename, tempdiscoveryservers, tempaddition)
		c.lker.Unlock()
		go c.start(addr)
	}
}
func (c *Client) notice() {
	for {
		data := <-c.noticech
		appuniquename := fmt.Sprintf("%s:%s", c.appname, data.PeerAddr)
		c.lker.Lock()
		server, ok := c.servers[appuniquename]
		if ok && data.Status {
			//this peer exist,it register on another discovery server
			server.lker.Lock()
			c.lker.Unlock()
			if _, ok := server.discoveryserver[data.DiscoveryServer]; ok {
				//already registered on this discovery server
				//this is impossible
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s duplicate register on discoveryserver:%s\n", appuniquename, data.DiscoveryServer)
			} else if !bytes.Equal(server.pickinfo.Addition, data.Addition) {
				//register with different registerinfo
				//this is impossible
				server.discoveryserver[data.DiscoveryServer] = struct{}{}
				server.pickinfo.DiscoveryServers = len(server.discoveryserver)
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s addition info conflict\n", appuniquename)
			} else {
				server.discoveryserver[data.DiscoveryServer] = struct{}{}
				server.pickinfo.DiscoveryServers = len(server.discoveryserver)
			}
			server.lker.Unlock()
		} else if ok {
			//this peer exist,it unregister on a discovery server
			server.lker.Lock()
			c.lker.Unlock()
			if _, ok := server.discoveryserver[data.DiscoveryServer]; !ok {
				//didn't registered on this discovery server before
				//this is impossible
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s duplicate unregister on discoveryserver:%s\n", appuniquename, data.DiscoveryServer)
			} else if !bytes.Equal(server.pickinfo.Addition, data.Addition) {
				//this is impossible
				delete(server.discoveryserver, data.DiscoveryServer)
				server.pickinfo.DiscoveryServers = len(server.discoveryserver)
				server.pickinfo.DiscoveryServerOfflineTime = time.Now().Unix()
				fmt.Printf("[Mrpc.client.notice.impossible]app:%s addition info conflict\n", appuniquename)
			} else {
				delete(server.discoveryserver, data.DiscoveryServer)
				server.pickinfo.DiscoveryServers = len(server.discoveryserver)
				server.pickinfo.DiscoveryServerOfflineTime = time.Now().Unix()
			}
			needoffline := false
			if len(server.discoveryserver) == 0 && server.status == 0 {
				needoffline = true
			} else if len(server.discoveryserver) != 0 && server.status == 0 {
				server.status = 1
				go c.start(data.PeerAddr)
			}
			if needoffline {
				//all req failed
				for _, req := range server.reqs {
					if req.callid != 0 {
						req.resp = nil
						req.err = Errmaker(ERRCLOSING, ERRMESSAGE[ERRCLOSING])
						req.finish <- struct{}{}
					}
				}
				server.reqs = make(map[uint64]*reqinfo, 10)
			}
			server.lker.Unlock()
			if needoffline {
				c.unregister(appuniquename)
			}
		} else if data.Status {
			//this peer not exist,it register on a discovery server
			tempdiscoveryservers := make(map[string]struct{}, 2)
			tempdiscoveryservers[data.DiscoveryServer] = struct{}{}
			c.servers[appuniquename] = c.getserver(appuniquename, tempdiscoveryservers, data.Addition)
			c.lker.Unlock()
			go c.start(data.PeerAddr)
		} else {
			c.lker.Unlock()
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
		if needoffline {
			//all req failed
			for _, req := range server.reqs {
				if req.callid != 0 {
					req.resp = nil
					req.err = Errmaker(ERRCLOSING, ERRMESSAGE[ERRCLOSING])
					req.finish <- struct{}{}
				}
			}
			server.reqs = make(map[uint64]*reqinfo, 10)
		}
		server.lker.Unlock()
		if needoffline {
			c.unregister(appuniquename)
		}
	}
}
func (c *Client) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	c.lker.RLock()
	server, ok := c.servers[appuniquename]
	if !ok || server.peer != nil || server.starttime != 0 {
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
func (c *Client) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
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
		server.starttime = starttime
		server.status = 3
		p.SetData(unsafe.Pointer(server))
	} else {
		//this is impossible
		p.Close()
	}
	server.lker.Unlock()
}
func (c *Client) userfunc(p *stream.Peer, appuniquename string, data []byte, starttime uint64) {
	server := (*Serverinfo)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		fmt.Printf("[Mrpc.client.userfunc.impossible]unmarshal data error:%s\n", e)
		return
	}
	server.lker.Lock()
	req, ok := server.reqs[msg.Callid]
	if !ok {
		server.lker.Unlock()
		return
	}
	server.pickinfo.Activecalls = len(server.reqs)
	server.pickinfo.Cpu = msg.Cpu
	if req.callid == msg.Callid {
		server.pickinfo.Netlag = time.Now().UnixNano() - req.starttime
		req.resp = msg.Body
		req.err = msg.Error
		req.finish <- struct{}{}
	}
	server.lker.Unlock()
}
func (c *Client) offlinefunc(p *stream.Peer, appuniquename string) {
	server := (*Serverinfo)(p.GetData())
	server.lker.Lock()
	server.peer = nil
	server.starttime = 0
	needoffline := len(server.discoveryserver) == 0
	if needoffline {
		server.status = 0
	} else {
		server.status = 1
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
	if needoffline {
		//all req failed
		for _, req := range server.reqs {
			if req.callid != 0 {
				req.resp = nil
				req.err = Errmaker(ERRCLOSING, ERRMESSAGE[ERRCLOSING])
				req.finish <- struct{}{}
			}
		}
		server.reqs = make(map[uint64]*reqinfo, 10)
	}
	server.lker.Unlock()
	if needoffline {
		c.unregister(appuniquename)
	}
}

func (c *Client) Call(ctx context.Context, path string, req []byte) ([]byte, *MsgErr) {
	//make mrpc system message
	msg := &Msg{
		Callid: atomic.AddUint64(&c.callid, 1),
		Path:   path,
	}
	dl, ok := ctx.Deadline()
	if ok {
		msg.Deadline = dl.UnixNano()
	}
	traceid := trace.GetTrace(ctx)
	if traceid == "" {
		traceid = trace.MakeTrace()
	}
	traceid = trace.AppendTrace(traceid, c.c.SelfName)
	msg.Trace = traceid
	msg.Body = req
	msg.Metadata = GetAllMetadata(ctx)
	d, _ := proto.Marshal(msg)
	if len(d) >= 65535 {
		return nil, Errmaker(ERRLARGE, ERRMESSAGE[ERRLARGE])
	}
	var server *Serverinfo
	r := c.getreq(msg.Callid)
	//pick server
	for {
		c.lker.RLock()
		if len(c.servers) == 0 {
			c.lker.RUnlock()
			c.putreq(r)
			return nil, Errmaker(ERRNOSERVER, ERRMESSAGE[ERRNOSERVER])
		}
		if len(c.servers) == 1 {
			for _, v := range c.servers {
				server = v
				break
			}
			if server.status != 3 {
				c.lker.RUnlock()
				c.putreq(r)
				return nil, Errmaker(ERRNOSERVER, ERRMESSAGE[ERRNOSERVER])
			}
		} else {
			pickinfo := make(map[*Serverinfo]PickInfo, int(float64(len(c.servers))*1.3))
			for _, server := range c.servers {
				if server.status != 3 {
					continue
				}
				pickinfo[server] = *server.pickinfo
			}
			if len(pickinfo) == 0 {
				c.lker.RUnlock()
				c.putreq(r)
				return nil, Errmaker(ERRNOSERVER, ERRMESSAGE[ERRNOSERVER])
			}
			if len(pickinfo) == 1 {
				for k := range pickinfo {
					server = k
					break
				}
			} else {
				server = c.pick(pickinfo)
				if server == nil {
					c.lker.RUnlock()
					c.putreq(r)
					return nil, Errmaker(ERRNOSERVER, ERRMESSAGE[ERRNOSERVER])
				}
			}
		}
		server.lker.Lock()
		c.lker.RUnlock()
		if server.status != 3 {
			server.lker.Unlock()
			continue
		}
		server.reqs[msg.Callid] = r
		if e := server.peer.SendMessage(d, server.starttime); e != nil {
			server.status = 4
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			continue
		}
		server.lker.Unlock()
		break
	}
	select {
	case <-r.finish:
		resp := r.resp
		err := r.err
		server.lker.Lock()
		delete(server.reqs, msg.Callid)
		c.putreq(r)
		server.lker.Unlock()
		//resp and err maybe both nil
		return resp, err
	case <-ctx.Done():
		server.lker.Lock()
		delete(server.reqs, msg.Callid)
		c.putreq(r)
		server.lker.Unlock()
		return nil, Errmaker(ERRCTXCANCEL, ERRMESSAGE[ERRCTXCANCEL])
	}
}

type reqinfo struct {
	callid    uint64
	finish    chan struct{}
	resp      []byte
	err       *MsgErr
	starttime int64
}

func (r *reqinfo) reset() {
	r.callid = 0
	for len(r.finish) > 0 {
		<-r.finish
	}
	r.resp = nil
	r.err = nil
	r.starttime = 0
}
func (c *Client) getreq(callid uint64) *reqinfo {
	r, ok := c.reqpool.Get().(*reqinfo)
	if ok {
		r.reset()
		r.callid = callid
		r.starttime = time.Now().UnixNano()
		return r
	}
	return &reqinfo{
		callid:    callid,
		finish:    make(chan struct{}),
		resp:      nil,
		err:       nil,
		starttime: time.Now().UnixNano(),
	}
}
func (c *Client) putreq(r *reqinfo) {
	r.reset()
	c.reqpool.Put(r)
}
