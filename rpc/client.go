package rpc

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	cerror "github.com/chenjie199234/Corelib/util/error"

	"google.golang.org/protobuf/proto"
)

type PickHandler func(servers []*ServerForPick) *ServerForPick
type DiscoveryHandler func(group, name string, client *RpcClient)

type ClientConfig struct {
	ConnTimeout            time.Duration
	GlobalTimeout          time.Duration //global timeout for every rpc call
	HeartTimeout           time.Duration
	HeartPorbe             time.Duration
	GroupNum               uint
	SocketRBuf             uint
	SocketWBuf             uint
	MaxMsgLen              uint
	MaxBufferedWriteMsgNum uint
	VerifyData             string
	Picker                 PickHandler
	Discover               DiscoveryHandler
}

func (c *ClientConfig) validate() {
	if c.ConnTimeout <= 0 {
		c.ConnTimeout = time.Millisecond * 500
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartTimeout <= 0 {
		c.HeartTimeout = 5 * time.Second
	}
	if c.HeartPorbe <= 0 {
		c.HeartPorbe = 1500 * time.Millisecond
	}
	if c.GroupNum == 0 {
		c.GroupNum = 1
	}
	if c.SocketRBuf == 0 {
		c.SocketRBuf = 1024
	}
	if c.SocketRBuf > 65535 {
		c.SocketRBuf = 65535
	}
	if c.SocketWBuf == 0 {
		c.SocketWBuf = 1024
	}
	if c.SocketWBuf > 65535 {
		c.SocketWBuf = 65535
	}
	if c.MaxMsgLen < 1024 {
		c.MaxMsgLen = 65535
	}
	if c.MaxMsgLen > 65535 {
		c.MaxMsgLen = 65535
	}
	if c.MaxBufferedWriteMsgNum == 0 {
		c.MaxBufferedWriteMsgNum = 256
	}
	if c.Picker == nil {
		c.Picker = defaultPicker
	}
	if c.Discover == nil {
		c.Discover = defaultDiscover
	}
}

//appuniquename = appname:addr
type RpcClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	instance    *stream.Instance

	lker    *sync.RWMutex
	servers []*ServerForPick

	callid  uint64
	reqpool *sync.Pool
}

type ServerForPick struct {
	addr             string
	discoveryservers []string //this app registered on which discovery server
	lker             *sync.Mutex
	peer             *stream.Peer
	starttime        int64
	status           int //0-idle,1-start,2-verify,3-connected,4-closing

	//active calls
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nanosecond
	Activecalls    int32  //current active calls
	DServers       int32  //this app registered on how many discoveryservers
	DServerOffline int64  //
	Addition       []byte //addition info register on register center
}

func (s *ServerForPick) Pickable() bool {
	return s.status == 3
}

func NewRpcClient(c *ClientConfig, selfgroup, selfname, group, name string) (*RpcClient, error) {
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	appname := group + "." + name
	if e := common.NameCheck(appname, true, true, false, true); e != nil {
		return nil, e
	}
	selfappname := selfgroup + "." + selfname
	if e := common.NameCheck(selfappname, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ClientConfig{}
	}
	c.validate()
	client := &RpcClient{
		selfappname: selfappname,
		appname:     appname,
		c:           c,
		lker:        &sync.RWMutex{},
		servers:     make([]*ServerForPick, 0, 10),
		callid:      0,
		reqpool:     &sync.Pool{},
	}
	dupc := &stream.InstanceConfig{
		HeartbeatTimeout:   c.HeartTimeout,
		HeartprobeInterval: c.HeartPorbe,
		GroupNum:           c.GroupNum,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:         c.ConnTimeout,
			SocketRBufLen:          c.SocketRBuf,
			SocketWBufLen:          c.SocketWBuf,
			MaxMsgLen:              c.MaxMsgLen,
			MaxBufferedWriteMsgNum: c.MaxBufferedWriteMsgNum,
		},
	}
	//tcp instalce
	dupc.Verifyfunc = client.verifyfunc
	dupc.Onlinefunc = client.onlinefunc
	dupc.Userdatafunc = client.userfunc
	dupc.Offlinefunc = client.offlinefunc
	client.instance, _ = stream.NewInstance(dupc, selfgroup, selfname)
	log.Info("[rpc.client] start finding server", group+"."+name, "with verifydata:", c.VerifyData)
	go c.Discover(group, name, client)
	return client, nil
}

//all:
//key:addr
//value:discovery servers
//addition
//addition info
func (c *RpcClient) UpdateDiscovery(all map[string][]string, addition []byte) {
	//offline app
	c.lker.Lock()
	defer c.lker.Unlock()
	for _, server := range c.servers {
		if _, ok := all[server.addr]; !ok {
			//this app unregistered
			server.discoveryservers = nil
			server.Pickinfo.DServers = 0
			server.Pickinfo.DServerOffline = time.Now().Unix()
		}
	}
	//online app or update app's discoveryservers
	for addr, discoveryservers := range all {
		var exist *ServerForPick
		for _, existserver := range c.servers {
			if existserver.addr == addr {
				exist = existserver
				break
			}
		}
		if exist == nil {
			//this is a new register
			c.servers = append(c.servers, &ServerForPick{
				addr:             addr,
				discoveryservers: discoveryservers,
				peer:             nil,
				starttime:        0,
				status:           1,
				reqs:             make(map[uint64]*req, 10),
				lker:             &sync.Mutex{},
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServers:       int32(len(discoveryservers)),
					DServerOffline: 0,
					Addition:       addition,
				},
			})
			go c.start(addr)
			continue
		}
		//this is not a new register
		//unregister on which discovery server
		head := 0
		tail := len(exist.discoveryservers) - 1
		for head <= tail {
			existdserver := exist.discoveryservers[head]
			find := false
			for _, newdserver := range discoveryservers {
				if newdserver == existdserver {
					find = true
					break
				}
			}
			if find {
				head++
			} else {
				if head != tail {
					exist.discoveryservers[head], exist.discoveryservers[tail] = exist.discoveryservers[tail], exist.discoveryservers[head]
				}
				tail--
			}
		}
		if exist.discoveryservers != nil && head != len(exist.discoveryservers) {
			exist.Pickinfo.DServerOffline = time.Now().UnixNano()
			exist.discoveryservers = exist.discoveryservers[:head]
		}
		if exist.discoveryservers == nil {
			exist.discoveryservers = make([]string, 0, 5)
		}
		//register on which new discovery server
		for _, newdserver := range discoveryservers {
			find := false
			for _, existdserver := range exist.discoveryservers {
				if existdserver == newdserver {
					find = true
					break
				}
			}
			if !find {
				exist.discoveryservers = append(exist.discoveryservers, newdserver)
				exist.Pickinfo.DServerOffline = 0
			}
		}
		exist.Pickinfo.Addition = addition
		exist.Pickinfo.DServers = int32(len(exist.discoveryservers))
	}
}
func (c *RpcClient) start(addr string) {
	tempverifydata := c.c.VerifyData + "|" + c.appname
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
		c.lker.RLock()
		var exist *ServerForPick
		for _, existserver := range c.servers {
			if existserver.addr == addr {
				exist = existserver
				break
			}
		}
		if exist == nil {
			//app removed
			c.lker.RUnlock()
			return
		}
		if len(exist.discoveryservers) == 0 {
			exist.lker.Lock()
			c.lker.RUnlock()
			exist.status = 0
			exist.lker.Unlock()
			c.unregister(addr)
		} else {
			exist.lker.Lock()
			c.lker.RUnlock()
			exist.status = 1
			exist.lker.Unlock()
			time.Sleep(100 * time.Millisecond)
			go c.start(addr)
		}
	}
}
func (c *RpcClient) unregister(addr string) {
	c.lker.Lock()
	var exist *ServerForPick
	var index int
	for i, existserver := range c.servers {
		if existserver.addr == addr {
			exist = existserver
			index = i
			break
		}
	}
	if exist == nil {
		//already removed
		c.lker.Unlock()
		return
	}
	//check again
	exist.lker.Lock()
	if len(exist.discoveryservers) == 0 && exist.status == 0 {
		//remove app
		c.servers[index], c.servers[len(c.servers)-1] = c.servers[len(c.servers)-1], c.servers[index]
		c.servers = c.servers[:len(c.servers)-1]
		exist.lker.Unlock()
		c.lker.Unlock()
	} else if len(exist.discoveryservers) != 0 && exist.status == 0 {
		exist.status = 1
		exist.lker.Unlock()
		c.lker.Unlock()
		time.Sleep(100 * time.Millisecond)
		go c.start(addr)
	}
}
func (c *RpcClient) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	if common.Byte2str(peerVerifyData) != c.c.VerifyData {
		return nil, false
	}
	c.lker.RLock()
	var exist *ServerForPick
	for _, existserver := range c.servers {
		if existserver.addr == appuniquename[strings.Index(appuniquename, ":")+1:] {
			exist = existserver
			break
		}
	}
	if exist == nil {
		//this is impossible
		c.lker.RUnlock()
		return nil, false
	}
	exist.lker.Lock()
	c.lker.RUnlock()
	if exist.peer != nil || exist.starttime != 0 || exist.status != 1 {
		exist.lker.Unlock()
		return nil, false
	}
	exist.status = 2
	exist.lker.Unlock()
	return nil, true
}
func (c *RpcClient) onlinefunc(p *stream.Peer, appuniquename string, starttime int64) {
	c.lker.RLock()
	var exist *ServerForPick
	for _, existserver := range c.servers {
		if existserver.addr == appuniquename[strings.Index(appuniquename, ":")+1:] {
			exist = existserver
			break
		}
	}
	if exist == nil {
		//this is impossible
		p.Close(starttime)
		c.lker.RUnlock()
		return
	}
	exist.lker.Lock()
	c.lker.RUnlock()
	if exist.peer != nil || exist.starttime != 0 || exist.status != 2 {
		p.Close(starttime)
		exist.lker.Unlock()
		return
	}
	exist.peer = p
	exist.starttime = starttime
	exist.status = 3
	p.SetData(unsafe.Pointer(exist))
	log.Info("[rpc.client.onlinefunc] server:", appuniquename, "online")
	exist.lker.Unlock()
}

func (c *RpcClient) userfunc(p *stream.Peer, appuniquename string, data []byte, starttime int64) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error("[rpc.client.userfunc] server:", appuniquename, "data format error:", e)
		return
	}
	server.lker.Lock()
	e := cerror.ErrorstrToError(msg.Error)
	if e != nil && e.Code == ERRCLOSING.Code {
		server.status = 4
	}
	req, ok := server.reqs[msg.Callid]
	if !ok {
		server.lker.Unlock()
		return
	}
	if req.callid == msg.Callid {
		req.resp = msg.Body
		req.err = e
		req.finish <- struct{}{}
	}
	server.lker.Unlock()
}

//func (c *RpcClient) offlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
func (c *RpcClient) offlinefunc(p *stream.Peer, appuniquename string) {
	server := (*ServerForPick)(p.GetData())
	if server == nil {
		return
	}
	log.Info("[rpc.client.offlinefunc] server:", appuniquename, "offline")
	server.lker.Lock()
	server.peer = nil
	server.starttime = 0
	//all req failed
	for _, req := range server.reqs {
		if req.callid != 0 {
			req.resp = nil
			req.err = ERRCLOSED
			req.finish <- struct{}{}
		}
	}
	server.reqs = make(map[uint64]*req, 10)
	if len(server.discoveryservers) == 0 {
		server.status = 0
		server.lker.Unlock()
		c.unregister(appuniquename)
	} else {
		server.status = 1
		server.lker.Unlock()
		time.Sleep(100 * time.Millisecond)
		go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
	}
}

func (c *RpcClient) Call(ctx context.Context, functimeout time.Duration, path string, in []byte, metadata map[string]string) ([]byte, error) {
	var min time.Duration
	if c.c.GlobalTimeout != 0 {
		min = c.c.GlobalTimeout
	}
	if functimeout != 0 {
		if min == 0 {
			min = functimeout
		} else if functimeout < min {
			min = functimeout
		}
	}
	if min != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, min)
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
		//ttl + server logic time
		return nil, ERRCTXTIMEOUT
	}
	msg := &Msg{
		Callid:   atomic.AddUint64(&c.callid, 1),
		Path:     path,
		Deadline: dl.UnixNano(),
		Body:     in,
		Metadata: metadata,
	}
	d, _ := proto.Marshal(msg)
	if len(d) > int(c.c.MaxMsgLen) {
		return nil, ERRREQMSGLARGE
	}
	var server *ServerForPick
	r := c.getreq(msg.Callid)
	for {
		for {
			//pick server
			c.lker.RLock()
			server = c.c.Picker(c.servers)
			if server == nil {
				c.lker.RUnlock()
				c.putreq(r)
				return nil, ERRNOSERVER
			}
			server.lker.Lock()
			c.lker.RUnlock()
			if !server.Pickable() {
				server.lker.Unlock()
				continue
			}
			//check timeout
			if msg.Deadline != 0 && msg.Deadline <= time.Now().UnixNano()+int64(5*time.Millisecond) {
				server.lker.Unlock()
				return nil, ERRCTXTIMEOUT
			}
			//send message
			if e := server.peer.SendMessage(d, server.starttime, false); e != nil {
				if e == stream.ERRMSGLENGTH {
					server.lker.Unlock()
					return nil, ERRREQMSGLARGE
				}
				if e == stream.ERRCONNCLOSED {
					server.status = 4
				}
				server.lker.Unlock()
				continue
			}
			//send message success,store req,add req num
			server.reqs[msg.Callid] = r
			atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
			server.lker.Unlock()
			break
		}
		select {
		case <-r.finish:
			//req finished,delete req,reduce req num
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			if r.err != nil {
				//req error,update last fail time
				atomic.StoreInt64(&server.Pickinfo.Lastfail, time.Now().UnixNano())
				if r.err.Code == ERRCLOSING.Code {
					//server is closing,this req can be retry
					r.resp = nil
					r.err = nil
					continue
				}
			}
			resp := r.resp
			err := r.err
			c.putreq(r)
			//resp and err maybe both nil
			return resp, err
		case <-ctx.Done():
			//req canceled or timeout,delete req,reduce req num
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			//update last fail time
			atomic.StoreInt64(&server.Pickinfo.Lastfail, time.Now().UnixNano())
			c.putreq(r)
			if ctx.Err() == context.Canceled {
				return nil, ERRCTXCANCEL
			} else if ctx.Err() == context.DeadlineExceeded {
				return nil, ERRCTXTIMEOUT
			} else {
				return nil, ERRUNKNOWN
			}
		}
	}
}

type req struct {
	callid    uint64
	finish    chan struct{}
	resp      []byte
	err       *cerror.Error
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
func (c *RpcClient) getreq(callid uint64) *req {
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
func (c *RpcClient) putreq(r *req) {
	r.reset()
	c.reqpool.Put(r)
}
