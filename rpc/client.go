package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type PickHandler func(servers map[string]*ServerForPick) *ServerForPick
type DiscoveryHandler func(group, name string, manually <-chan struct{}, client *RpcClient)

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
}

//appuniquename = appname:addr
type RpcClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	instance    *stream.Instance

	lker         *sync.RWMutex
	servers      map[string]*ServerForPick //key server addr
	addrdata     []byte
	manually     chan struct{}
	manualNotice map[chan struct{}]struct{}
	mlker        *sync.Mutex

	callid  uint64
	reqpool *sync.Pool
}

type ServerForPick struct {
	addr     string
	dservers map[string]struct{} //this app registered on which discovery server
	lker     *sync.Mutex
	peer     *stream.Peer
	sid      int64
	status   int //0-idle,1-start,2-verify,3-connected,4-closing

	//active calls
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *pickinfo
}
type pickinfo struct {
	Lastfail       int64  //last fail timestamp nanosecond
	Activecalls    int32  //current active calls
	DServerNum     int32  //this app registered on how many discoveryservers
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
		return nil, errors.New("[rpc.client] missing config")
	}
	if c.Discover == nil {
		return nil, errors.New("[rpc.client] missing discover in config")
	}
	if c.Picker == nil {
		log.Warning("[rpc.client] missing picker in config,default picker will be used")
		c.Picker = defaultPicker
	}
	c.validate()
	client := &RpcClient{
		selfappname:  selfappname,
		appname:      appname,
		c:            c,
		lker:         &sync.RWMutex{},
		servers:      make(map[string]*ServerForPick, 10),
		addrdata:     nil,
		manually:     make(chan struct{}, 1),
		manualNotice: make(map[chan struct{}]struct{}, 100),
		mlker:        &sync.Mutex{},
		callid:       0,
		reqpool:      &sync.Pool{},
	}
	dupc := &stream.InstanceConfig{
		HeartbeatTimeout:       c.HeartTimeout,
		HeartprobeInterval:     c.HeartPorbe,
		MaxBufferedWriteMsgNum: c.MaxBufferedWriteMsgNum,
		GroupNum:               c.GroupNum,
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnTimeout,
			SocketRBufLen:  c.SocketRBuf,
			SocketWBufLen:  c.SocketWBuf,
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	//tcp instalce
	dupc.Verifyfunc = client.verifyfunc
	dupc.Onlinefunc = client.onlinefunc
	dupc.Userdatafunc = client.userfunc
	dupc.Offlinefunc = client.offlinefunc
	client.instance, _ = stream.NewInstance(dupc, selfgroup, selfname)
	log.Info("[rpc.client] start finding server", group+"."+name, "with verifydata:", c.VerifyData)
	go c.Discover(group, name, client.manually, client)
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr
func (c *RpcClient) UpdateDiscovery(all map[string]*RegisterData) {
	//check need update
	addrdata, _ := json.Marshal(all)
	c.lker.Lock()
	defer func() {
		c.mlker.Lock()
		for notice := range c.manualNotice {
			notice <- struct{}{}
			delete(c.manualNotice, notice)
		}
		c.mlker.Unlock()
		c.lker.Unlock()
	}()
	if bytes.Equal(c.addrdata, addrdata) {
		return
	}
	c.addrdata = addrdata
	//offline app
	for _, server := range c.servers {
		if _, ok := all[server.addr]; !ok {
			//this app unregistered
			server.dservers = nil
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().Unix()
		}
	}
	//online app or update app's dservers
	for addr, registerdata := range all {
		exist, ok := c.servers[addr]
		if !ok {
			//this is a new register
			c.servers[addr] = &ServerForPick{
				addr:     addr,
				dservers: registerdata.DServers,
				peer:     nil,
				sid:      0,
				status:   1,
				reqs:     make(map[uint64]*req, 10),
				lker:     &sync.Mutex{},
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
			}
			go c.start(addr)
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range exist.dservers {
				if _, ok := registerdata.DServers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range registerdata.DServers {
				if _, ok := exist.dservers[dserver]; !ok {
					exist.Pickinfo.DServerOffline = 0
					break
				}
			}
			exist.dservers = registerdata.DServers
			exist.Pickinfo.Addition = registerdata.Addition
			exist.Pickinfo.DServerNum = int32(len(registerdata.DServers))
		}
	}
}
func (c *RpcClient) start(addr string) {
	tempverifydata := c.c.VerifyData + "|" + c.appname
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
		c.lker.RLock()
		exist, ok := c.servers[addr]
		if !ok {
			//app removed
			c.lker.RUnlock()
			return
		}
		if len(exist.dservers) == 0 {
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
			//can't connect to the server,but the server was registered on some dservers
			//we need to triger manually update dserver data,to make sure this server is alive
			select {
			case c.manually <- struct{}{}:
			default:
			}
			time.Sleep(100 * time.Millisecond)
			exist.lker.Lock()
			if len(exist.dservers) != 0 {
				go c.start(addr)
				exist.lker.Unlock()
			} else {
				exist.status = 1
				exist.lker.Unlock()
				c.unregister(addr)
			}
		}
	}
}
func (c *RpcClient) unregister(addr string) {
	c.lker.Lock()
	exist, ok := c.servers[addr]
	if !ok {
		//already removed
		c.lker.Unlock()
		return
	}
	//check again
	exist.lker.Lock()
	if len(exist.dservers) == 0 {
		//remove app
		delete(c.servers, addr)
		exist.lker.Unlock()
		c.lker.Unlock()
	} else {
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
	if exist.peer != nil || exist.sid != 0 || exist.status != 1 {
		exist.lker.Unlock()
		return nil, false
	}
	exist.status = 2
	exist.lker.Unlock()
	return nil, true
}
func (c *RpcClient) onlinefunc(p *stream.Peer, appuniquename string, sid int64) {
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
		p.Close(sid)
		c.lker.RUnlock()
		return
	}
	exist.lker.Lock()
	c.lker.RUnlock()
	if exist.peer != nil || exist.sid != 0 || exist.status != 2 {
		p.Close(sid)
		exist.lker.Unlock()
		return
	}
	exist.peer = p
	exist.sid = sid
	exist.status = 3
	p.SetData(unsafe.Pointer(exist))
	log.Info("[rpc.client.onlinefunc] server:", appuniquename, "online")
	exist.lker.Unlock()
}

func (c *RpcClient) userfunc(p *stream.Peer, appuniquename string, data []byte, sid int64) {
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

func (c *RpcClient) offlinefunc(p *stream.Peer, appuniquename string) {
	server := (*ServerForPick)(p.GetData())
	if server == nil {
		return
	}
	log.Info("[rpc.client.offlinefunc] server:", appuniquename, "offline")
	server.lker.Lock()
	server.peer = nil
	server.sid = 0
	//all req failed
	for _, req := range server.reqs {
		if req.callid != 0 {
			req.resp = nil
			req.err = ERRCLOSED
			req.finish <- struct{}{}
		}
	}
	server.reqs = make(map[uint64]*req, 10)
	if len(server.dservers) == 0 {
		server.status = 0
		server.lker.Unlock()
		c.unregister(appuniquename)
	} else {
		server.status = 1
		server.lker.Unlock()
		//disconnect to server,but the server was registered on some dservers
		//we need to triger manually update dserver data,to make sure this server is alive
		select {
		case c.manually <- struct{}{}:
		default:
		}
		time.Sleep(100 * time.Millisecond)
		server.lker.Lock()
		if len(server.dservers) != 0 {
			go c.start(appuniquename[strings.Index(appuniquename, ":")+1:])
			server.lker.Unlock()
		} else {
			server.status = 0
			server.lker.Unlock()
			c.unregister(appuniquename)
		}
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
		manual := false
		for {
			//pick server
			c.lker.RLock()
			server = c.c.Picker(c.servers)
			if server == nil {
				c.lker.RUnlock()
				if manual {
					c.putreq(r)
					return nil, ERRNOSERVER
				}
				c.mlker.Lock()
				manualNotice := make(chan struct{}, 1)
				c.manualNotice[manualNotice] = struct{}{}
				c.mlker.Unlock()
				//manually update server discover info
				select {
				case c.manually <- struct{}{}:
				default:
				}
				//wait manual update finish
				select {
				case <-manualNotice:
					manual = true
					continue
				case <-ctx.Done():
					c.mlker.Lock()
					delete(c.manualNotice, manualNotice)
					c.mlker.Unlock()
					if ctx.Err() == context.DeadlineExceeded {
						return nil, ERRCTXTIMEOUT
					} else if ctx.Err() == context.Canceled {
						return nil, ERRCTXCANCEL
					} else {
						return nil, ERRUNKNOWN
					}
				}
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
			if e := server.peer.SendMessage(d, server.sid, false); e != nil {
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
				server.Pickinfo.Lastfail = time.Now().UnixNano()
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
			server.Pickinfo.Lastfail = time.Now().UnixNano()
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
