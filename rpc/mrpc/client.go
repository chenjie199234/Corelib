package mrpc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/merror"
	"github.com/chenjie199234/Corelib/stream"
	//"github.com/chenjie199234/Corelib/sys/trace"

	"google.golang.org/protobuf/proto"
)

type PickHandler func(servers []*Serverapp) *Serverapp
type DiscoveryHandler func(appname string, client *MrpcClient)

//appuniquename = appname:addr
type MrpcClient struct {
	c          *stream.InstanceConfig
	appname    string
	verifydata []byte
	instance   *stream.Instance

	lker       *sync.RWMutex
	servers    []*Serverapp
	serverpool *sync.Pool

	callid  uint64
	reqpool *sync.Pool

	pick      PickHandler
	discovery DiscoveryHandler
}

type Serverapp struct {
	lker          *sync.Mutex
	appuniquename string
	//key discoveryserver uniquename
	discoveryserver map[string]struct{} //this app registered on which discovery server
	peer            *stream.Peer
	starttime       uint64
	status          int //0-idle,1-start,2-verify,3-connected,4-closing

	//active calls
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *PickInfo
}
type PickInfo struct {
	Cpu                        float64 //cpuinfo
	Netlag                     int64   //netlaginfo
	Activecalls                int     //current active calls
	DiscoveryServers           int     //this server registered on how many discoveryservers
	DiscoveryServerOfflineTime int64   //
	Addition                   []byte  //addition info register on register center
}

func (s *Serverapp) Pickable() bool {
	return s.status == 3
}
func (c *MrpcClient) getserver(appuniquename string, discoveryservers map[string]struct{}, addition []byte) *Serverapp {
	s, ok := c.serverpool.Get().(*Serverapp)
	if !ok {
		return &Serverapp{
			lker:            &sync.Mutex{},
			appuniquename:   appuniquename,
			discoveryserver: discoveryservers,
			peer:            nil,
			starttime:       0,
			status:          1,

			reqs: make(map[uint64]*req, 10),

			Pickinfo: &PickInfo{
				Cpu:              0,
				Netlag:           0,
				Activecalls:      0,
				DiscoveryServers: len(discoveryservers),
				Addition:         addition,
			},
		}
	}
	s.appuniquename = appuniquename
	s.discoveryserver = discoveryservers
	s.peer = nil
	s.starttime = 0
	s.status = 1

	s.reqs = make(map[uint64]*req, 10)

	s.Pickinfo.Cpu = 0
	s.Pickinfo.Netlag = 0
	s.Pickinfo.Activecalls = 0
	s.Pickinfo.DiscoveryServers = len(discoveryservers)
	s.Pickinfo.Addition = addition
	return s
}
func (c *MrpcClient) putserver(s *Serverapp) {
	s.appuniquename = ""
	s.discoveryserver = nil
	s.peer = nil
	s.starttime = 0
	s.status = 0

	s.reqs = make(map[uint64]*req, 10)

	s.Pickinfo.Cpu = 0
	s.Pickinfo.Netlag = 0
	s.Pickinfo.Activecalls = 0
	s.Pickinfo.DiscoveryServers = 0
	s.Pickinfo.Addition = nil
	c.serverpool.Put(s)
}

func NewMrpcClient(c *stream.InstanceConfig, appname string, vdata []byte, pick PickHandler, discovery DiscoveryHandler) *MrpcClient {
	//use default pick
	if pick == nil {
		pick = defaultPicker
	}
	if discovery == nil {
		discovery = defaultdiscovery
	}
	client := &MrpcClient{
		appname:    appname,
		verifydata: vdata,
		lker:       &sync.RWMutex{},
		servers:    make([]*Serverapp, 0, 10),
		serverpool: &sync.Pool{},
		callid:     0,
		reqpool:    &sync.Pool{},
		pick:       pick,
		discovery:  discovery,
	}
	//tcp instalce
	dupc := *c //duplicate to remove the callback func race
	dupc.Verifyfunc = client.verifyfunc
	dupc.Onlinefunc = client.onlinefunc
	dupc.Userdatafunc = client.userfunc
	dupc.Offlinefunc = client.offlinefunc
	client.c = &dupc
	client.instance = stream.NewInstance(&dupc)
	go discovery(appname, client)
	return client
}

//first key:addr
//second key:discovery server
//value:addition data
func (c *MrpcClient) UpdateDiscovery(allapps map[string]map[string][]byte) {
	//offline app
	c.lker.Lock()
	defer c.lker.Unlock()
	for _, server := range c.servers {
		addr := server.appuniquename[strings.Index(server.appuniquename, ":")+1:]
		if _, ok := allapps[addr]; !ok {
			//this app unregistered
			server.lker.Lock()
			server.discoveryserver = nil
			server.Pickinfo.DiscoveryServers = 0
			server.Pickinfo.DiscoveryServerOfflineTime = time.Now().Unix()
			server.lker.Unlock()
		}
	}
	//online app or update app's discoveryservers
	for addr, discoveryservers := range allapps {
		var server *Serverapp
		appuniquename := fmt.Sprintf("%s:%s", c.appname, addr)
		for _, tempserver := range c.servers {
			if tempserver.appuniquename == appuniquename {
				server = tempserver
				break
			}
		}
		if server != nil {
			//check disckvery servers
			onlyunregister := false
			server.lker.Lock()
			//check unregister from discovery server
			for discoveryserver := range server.discoveryserver {
				if _, ok := discoveryservers[discoveryserver]; !ok {
					delete(server.discoveryserver, discoveryserver)
					onlyunregister = true
				}
			}
			//check register on discovery server
			for discoveryserver, addition := range discoveryservers {
				if _, ok := server.discoveryserver[discoveryserver]; !ok {
					if bytes.Equal(addition, server.Pickinfo.Addition) {
						server.discoveryserver[discoveryserver] = struct{}{}
						onlyunregister = false
					} else {
						fmt.Printf("[Mrpc.client.UpdateDiscovery.impossible]app:%s addition data conflict\n", appuniquename)
					}
				}
			}
			server.Pickinfo.DiscoveryServers = len(server.discoveryserver)
			if onlyunregister {
				server.Pickinfo.DiscoveryServerOfflineTime = time.Now().Unix()
			}
			server.lker.Unlock()
		} else {
			//online new
			temp := make(map[string]struct{}, len(discoveryservers))
			var tempaddition []byte
			insert := true
			for discoveryserver, addition := range discoveryservers {
				temp[discoveryserver] = struct{}{}
				if tempaddition == nil {
					tempaddition = addition
				} else if bytes.Equal(tempaddition, addition) {
					fmt.Printf("[Mrpc.client.UpdateDiscovery.impossible]app:%s addition data conflict\n", appuniquename)
					insert = false
					break
				}
			}
			if insert {
				c.servers = append(c.servers, c.getserver(appuniquename, temp, tempaddition))
				go c.start(addr)
			}
		}
	}
}
func (c *MrpcClient) start(addr string) {
	tempverifydata := common.Byte2str(c.verifydata) + "|" + c.appname
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
		appuniquename := fmt.Sprintf("%s:%s", c.appname, addr)
		c.lker.RLock()
		var server *Serverapp
		for _, tempserver := range c.servers {
			if tempserver.appuniquename == appuniquename {
				server = tempserver
				break
			}
		}
		if server == nil {
			//app removed
			c.lker.RUnlock()
			return
		}
		server.lker.Lock()
		c.lker.RUnlock()
		if len(server.discoveryserver) == 0 {
			server.status = 0
		} else {
			server.status = 1
			go c.start(addr)
		}
		//all req failed,here would't have data
		if len(server.reqs) != 0 {
			fmt.Printf("[Mrpc.client.start.impossible]unconnected app:%s has request\n", appuniquename)
			for _, req := range server.reqs {
				if req.callid != 0 {
					req.resp = nil
					req.err = ERR[ERRCLOSED]
					req.finish <- struct{}{}
				}
			}
			server.reqs = make(map[uint64]*req, 10)
		}
		server.lker.Unlock()
		if server.status == 0 {
			c.unregister(appuniquename)
		}
	}
}
func (c *MrpcClient) unregister(appuniquename string) {
	c.lker.Lock()
	var server *Serverapp
	var index int
	for tempindex, tempserver := range c.servers {
		if tempserver.appuniquename == appuniquename {
			server = tempserver
			index = tempindex
			break
		}
	}
	if server == nil {
		//already removed
		c.lker.Unlock()
		return
	}
	//check again
	server.lker.Lock()
	if len(server.discoveryserver) == 0 && server.status == 0 {
		//remove app
		c.servers[index], c.servers[len(c.servers)-1] = c.servers[len(c.servers)-1], c.servers[index]
		c.servers = c.servers[:len(c.servers)-1]
		c.putserver(server)
	} else if len(server.discoveryserver) != 0 && server.status == 0 {
		server.status = 1
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
	server.lker.Unlock()
	c.lker.Unlock()
}
func (c *MrpcClient) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	c.lker.RLock()
	var server *Serverapp
	for _, tempserver := range c.servers {
		if tempserver.appuniquename == appuniquename {
			server = tempserver
			break
		}
	}
	if server == nil {
		//server offline
		c.lker.RUnlock()
		return nil, false
	}
	server.lker.Lock()
	c.lker.RUnlock()
	if server.peer != nil || server.starttime != 0 || server.status != 1 {
		//this is impossible
		server.lker.Unlock()
		fmt.Printf("[Mrpc.client.verifyfunc.impossible]server:%s conflict\n", appuniquename)
		return nil, false
	}
	server.status = 2
	server.lker.Unlock()
	return nil, true
}
func (c *MrpcClient) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	c.lker.RLock()
	var server *Serverapp
	for _, tempserver := range c.servers {
		if tempserver.appuniquename == appuniquename {
			server = tempserver
			break
		}
	}
	if server == nil {
		//server offline
		p.Close()
		c.lker.RUnlock()
		return
	}
	server.lker.Lock()
	c.lker.RUnlock()
	if server.status != 2 || server.peer != nil || server.starttime != 0 {
		//this is impossible
		p.Close()
		server.lker.Unlock()
		fmt.Printf("[Mrpc.client.onlinefunc.impossible]server:%s conflict\n", appuniquename)
		return
	}
	server.peer = p
	server.starttime = starttime
	server.status = 3
	p.SetData(unsafe.Pointer(server))
	fmt.Printf("[Mrpc.client.onlinefunc]server:%s online\n", appuniquename)
	server.lker.Unlock()
}

func (c *MrpcClient) userfunc(p *stream.Peer, appuniquename string, data []byte, starttime uint64) {
	server := (*Serverapp)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		fmt.Printf("[Mrpc.client.userfunc.impossible]unmarshal data error:%s\n", e)
		return
	}
	server.lker.Lock()
	e := merror.ErrorstrToMError(msg.Error)
	if e != nil && e.Code == ERRCLOSING {
		server.status = 4
	}
	req, ok := server.reqs[msg.Callid]
	if !ok {
		server.lker.Unlock()
		return
	}
	server.Pickinfo.Cpu = msg.Cpu
	if req.callid == msg.Callid {
		server.Pickinfo.Netlag = time.Now().UnixNano() - req.starttime
		req.resp = msg.Body
		req.err = e
		req.finish <- struct{}{}
	}
	server.lker.Unlock()
}
func (c *MrpcClient) offlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	server := (*Serverapp)(p.GetData())
	server.lker.Lock()
	server.peer = nil
	server.starttime = 0
	if len(server.discoveryserver) == 0 {
		server.status = 0
	} else {
		server.status = 1
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
	//all req failed
	for _, req := range server.reqs {
		if req.callid != 0 {
			req.resp = nil
			req.err = ERR[ERRCLOSED]
			req.finish <- struct{}{}
		}
	}
	server.reqs = make(map[uint64]*req, 10)
	fmt.Printf("[Mrpc.client.onlinefunc]app:%s offline\n", appuniquename)
	server.lker.Unlock()
	if server.status == 0 {
		c.unregister(appuniquename)
	}
}

func (c *MrpcClient) Call(ctx context.Context, path string, in []byte) ([]byte, error) {
	//make mrpc system message
	dl, ok := ctx.Deadline()
	if ok && dl.UnixNano() <= time.Now().UnixNano()+int64(time.Millisecond) {
		return nil, ERR[ERRCTXTIMEOUT]
	}
	msg := &Msg{
		Callid: atomic.AddUint64(&c.callid, 1),
		Path:   path,
	}
	if !dl.IsZero() {
		msg.Deadline = dl.UnixNano()
	}

	//traceid := trace.GetTrace(ctx)
	//if traceid == "" {
	//        traceid = trace.MakeTrace()
	//}
	//traceid = trace.AppendTrace(traceid, c.c.SelfName)
	//msg.Trace = traceid
	msg.Body = in
	msg.Metadata = GetAllMetadata(ctx)
	d, _ := proto.Marshal(msg)
	if len(d) > c.c.TcpC.MaxMessageLen {
		return nil, ERR[ERRLARGE]
	}
	var server *Serverapp
	r := c.getreq(msg.Callid)
	for {
		//pick server
		for {
			c.lker.RLock()
			if len(c.servers) == 0 {
				c.lker.RUnlock()
				c.putreq(r)
				return nil, ERR[ERRNOSERVER]
			}
			server = c.pick(c.servers)
			if server == nil {
				c.lker.RUnlock()
				c.putreq(r)
				return nil, ERR[ERRNOSERVER]
			}
			server.lker.Lock()
			c.lker.RUnlock()
			if !server.Pickable() {
				server.lker.Unlock()
				continue
			}
			server.reqs[msg.Callid] = r
			if msg.Deadline != 0 && msg.Deadline <= time.Now().UnixNano()+int64(time.Millisecond) {
				delete(server.reqs, msg.Callid)
				server.lker.Unlock()
				return nil, ERR[ERRCTXTIMEOUT]
			}
			if e := server.peer.SendMessage(d, server.starttime, true); e != nil {
				//the error can only be connection closed
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
			if r.err != nil && r.err.Code == ERRCLOSING {
				r.resp = nil
				r.err = nil
				continue
			}
			resp := r.resp
			err := r.err
			server.lker.Lock()
			_, ok := server.reqs[msg.Callid]
			if ok {
				delete(server.reqs, msg.Callid)
				server.Pickinfo.Activecalls = len(server.reqs)
			}
			c.putreq(r)
			server.lker.Unlock()
			//resp and err maybe both nil
			return resp, err
		case <-ctx.Done():
			server.lker.Lock()
			_, ok := server.reqs[msg.Callid]
			if ok {
				delete(server.reqs, msg.Callid)
				server.Pickinfo.Activecalls = len(server.reqs)
			}
			c.putreq(r)
			server.lker.Unlock()
			e := ctx.Err()
			if e == context.Canceled {
				return nil, ERR[ERRCTXCANCEL]
			} else if e == context.DeadlineExceeded {
				return nil, ERR[ERRCTXTIMEOUT]
			} else {
				return nil, ERR[ERRUNKNOWN]
			}
		}
	}
}

type req struct {
	callid    uint64
	finish    chan struct{}
	resp      []byte
	err       *merror.MError
	starttime int64
}

func (r *req) reset() {
	r.callid = 0
	for len(r.finish) > 0 {
		<-r.finish
	}
	r.resp = nil
	r.err = nil
	r.starttime = 0
}
func (c *MrpcClient) getreq(callid uint64) *req {
	r, ok := c.reqpool.Get().(*req)
	if ok {
		r.reset()
		r.callid = callid
		r.starttime = time.Now().UnixNano()
		return r
	}
	return &req{
		callid:    callid,
		finish:    make(chan struct{}),
		resp:      nil,
		err:       nil,
		starttime: time.Now().UnixNano(),
	}
}
func (c *MrpcClient) putreq(r *req) {
	r.reset()
	c.reqpool.Put(r)
}
