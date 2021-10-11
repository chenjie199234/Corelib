package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"google.golang.org/protobuf/proto"
)

//param's key is server's addr "ip:port"
type PickHandler func(servers map[string]*ServerForPick) *ServerForPick

//return data's key is server's addr "ip:port"
type DiscoveryHandler func(group, name string, manually <-chan struct{}) (map[string]*RegisterData, error)

type ClientConfig struct {
	ConnTimeout      time.Duration
	GlobalTimeout    time.Duration //global timeout for every rpc call
	HeartPorbe       time.Duration
	GroupNum         uint32
	SocketRBuf       uint32
	SocketWBuf       uint32
	MaxMsgLen        uint32
	VerifyData       string
	UseTLS           bool     //rpc or rpcs
	SkipVerifyTLS    bool     //don't verify the server's cert
	CAs              []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	DiscoverFunction DiscoveryHandler //this function will be called in for loop
}

func (c *ClientConfig) validate() {
	if c.ConnTimeout <= 0 {
		c.ConnTimeout = time.Millisecond * 500
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
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
}

//appuniquename = appname:addr
type RpcClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	tlsc        *tls.Config
	instance    *stream.Instance

	lker    *sync.RWMutex
	servers map[string]*ServerForPick //key server addr

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
	status   int //0-closed,1-start,2-verify,3-connected,4-closing

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
	if c.DiscoverFunction == nil {
		return nil, errors.New("[rpc.client] missing discover in config")
	}
	if c.Picker == nil {
		log.Warning(nil, "[rpc.client] missing picker in config,default picker will be used")
		c.Picker = defaultPicker
	}
	c.validate()
	var certpool *x509.CertPool
	if len(c.CAs) != 0 {
		certpool = x509.NewCertPool()
		for _, cert := range c.CAs {
			certPEM, e := os.ReadFile(cert)
			if e != nil {
				return nil, errors.New("[web.client] read cert file:" + cert + " error:" + e.Error())
			}
			if !certpool.AppendCertsFromPEM(certPEM) {
				return nil, errors.New("[web.client] load cert file:" + cert + " error:" + e.Error())
			}
		}
	}
	client := &RpcClient{
		selfappname: selfappname,
		appname:     appname,
		c:           c,
		tlsc: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},
		lker:         &sync.RWMutex{},
		servers:      make(map[string]*ServerForPick, 10),
		manually:     make(chan struct{}, 1),
		manualNotice: make(map[chan struct{}]struct{}, 100),
		mlker:        &sync.Mutex{},
		callid:       0,
		reqpool:      &sync.Pool{},
	}
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
		GroupNum:           c.GroupNum,
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnTimeout,
			SocketRBufLen:  c.SocketRBuf,
			SocketWBufLen:  c.SocketWBuf,
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	//tcp instalce
	instancec.Verifyfunc = client.verifyfunc
	instancec.Onlinefunc = client.onlinefunc
	instancec.Userdatafunc = client.userfunc
	instancec.Offlinefunc = client.offlinefunc
	client.instance, _ = stream.NewInstance(instancec, selfgroup, selfname)
	//init discover
	client.manually <- struct{}{}
	manualNotice := make(chan struct{}, 1)
	client.manualNotice[manualNotice] = struct{}{}
	go defaultDiscover(group, name, client)
	<-manualNotice
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr
func (c *RpcClient) updateDiscovery(all map[string]*RegisterData) {
	//check need update
	c.lker.Lock()
	defer c.lker.Unlock()
	//offline app
	for _, exist := range c.servers {
		exist.lker.Lock()
		if _, ok := all[exist.addr]; !ok {
			//this app unregistered
			exist.dservers = nil
			exist.Pickinfo.DServerNum = 0
			exist.Pickinfo.DServerOffline = time.Now().Unix()
		}
		exist.lker.Unlock()
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
			exist.lker.Lock()
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
			exist.lker.Unlock()
		}
	}
}
func (c *RpcClient) start(addr string) {
	tempverifydata := c.c.VerifyData + "|" + c.appname
	var tlsc *tls.Config
	if c.c.UseTLS {
		tlsc = c.tlsc
	}
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata), tlsc); r == "" {
		c.lker.RLock()
		exist, ok := c.servers[addr]
		if !ok {
			//app removed
			c.lker.RUnlock()
			return
		}
		exist.lker.Lock()
		c.lker.RUnlock()
		if len(exist.dservers) == 0 {
			exist.status = 0
			exist.lker.Unlock()
			c.unregister(addr)
		} else {
			exist.status = 1
			exist.lker.Unlock()
			//can't connect to the server,but the server was registered on some dservers
			//we need to triger manually update dserver data,to make sure this server is alive
			manualNotice := make(chan struct{}, 1)
			c.mlker.Lock()
			c.manualNotice[manualNotice] = struct{}{}
			if len(c.manualNotice) == 1 {
				c.manually <- struct{}{}
			}
			c.mlker.Unlock()
			<-manualNotice
			exist.lker.Lock()
			if len(exist.dservers) == 0 {
				exist.status = 0
				exist.lker.Unlock()
				c.unregister(addr)
			} else {
				go c.start(addr)
				exist.lker.Unlock()
			}
		}
	}
}
func (c *RpcClient) unregister(addr string) {
	c.lker.Lock()

	defer c.lker.Unlock()
	exist, ok := c.servers[addr]
	if !ok {
		//already removed
		return
	}
	//check again
	exist.lker.Lock()
	if len(exist.dservers) == 0 {
		//remove app
		delete(c.servers, addr)
		exist.lker.Unlock()
	} else {
		exist.status = 1
		go c.start(addr)
		exist.lker.Unlock()
	}
}
func (c *RpcClient) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	if common.Byte2str(peerVerifyData) != c.c.VerifyData {
		return nil, false
	}
	//verify success,update status
	c.lker.RLock()
	addr := appuniquename[strings.Index(appuniquename, ":")+1:]
	exist, ok := c.servers[addr]
	if !ok {
		//this is impossible
		c.lker.RUnlock()
		return nil, false
	}
	exist.lker.Lock()
	c.lker.RUnlock()
	exist.status = 2
	exist.lker.Unlock()
	return nil, true
}
func (c *RpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	c.lker.RLock()
	addr := p.GetRemoteAddr()
	exist, ok := c.servers[addr]
	if !ok {
		//this is impossible
		c.lker.RUnlock()
		return false
	}
	exist.lker.Lock()
	c.lker.RUnlock()
	exist.peer = p
	exist.status = 3
	p.SetData(unsafe.Pointer(exist))
	log.Info(nil, "[rpc.client.onlinefunc] server:", p.GetPeerUniqueName(), "online")
	exist.lker.Unlock()
	return true
}

func (c *RpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[rpc.client.userfunc] server:", p.GetPeerUniqueName(), "data format error:", e)
		return
	}
	server.lker.Lock()
	if msg.Error != nil && msg.Error.Code == ERRCLOSING.Code {
		server.status = 4
	}
	req, ok := server.reqs[msg.Callid]
	if !ok {
		server.lker.Unlock()
		return
	}
	req.resp = msg.Body
	req.err = msg.Error
	req.finish <- struct{}{}
	server.lker.Unlock()
}

func (c *RpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	if server == nil {
		return
	}
	log.Info(nil, "[rpc.client.offlinefunc] server:", p.GetPeerUniqueName(), "offline")
	server.lker.Lock()
	server.peer = nil
	//all req failed
	for _, req := range server.reqs {
		req.resp = nil
		req.err = ERRCLOSED
		req.finish <- struct{}{}
	}
	server.reqs = make(map[uint64]*req, 10)
	if len(server.dservers) == 0 {
		server.status = 0
		server.lker.Unlock()
		c.unregister(p.GetRemoteAddr())
	} else {
		server.status = 1
		server.lker.Unlock()
		//disconnect to server,but the server was registered on some dservers
		//we need to triger manually update dserver data,to make sure this server is alive
		manualNotice := make(chan struct{}, 1)
		c.mlker.Lock()
		c.manualNotice[manualNotice] = struct{}{}
		if len(c.manualNotice) == 1 {
			c.manually <- struct{}{}
		}
		c.mlker.Unlock()
		<-manualNotice
		server.lker.Lock()
		if len(server.dservers) == 0 {
			server.status = 0
			server.lker.Unlock()
			c.unregister(p.GetRemoteAddr())
		} else {
			go c.start(p.GetRemoteAddr())
			server.lker.Unlock()
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
		return nil, cerror.ErrDeadlineExceeded
	}
	msg := &Msg{
		Callid:   atomic.AddUint64(&c.callid, 1),
		Path:     path,
		Body:     in,
		Metadata: metadata,
	}
	if ok {
		msg.Deadline = dl.UnixNano()
	}
	traceid, _, _, selfmethod, selfpath := trace.GetTrace(ctx)
	if traceid != "" {
		msg.Tracedata = map[string]string{
			"Traceid":      traceid,
			"SourceMethod": selfmethod,
			"SourcePath":   selfpath,
		}
	}
	d, _ := proto.Marshal(msg)
	r := c.getreq(msg.Callid)
	manual := false
	for {
		var server *ServerForPick
		//pick server
		c.lker.RLock()
		server = c.c.Picker(c.servers)
		if server == nil {
			c.lker.RUnlock()
			if manual {
				c.putreq(r)
				return nil, ERRNOSERVER
			}
			manualNotice := make(chan struct{}, 1)
			c.mlker.Lock()
			c.manualNotice[manualNotice] = struct{}{}
			if len(c.manualNotice) == 1 {
				c.manually <- struct{}{}
			}
			c.mlker.Unlock()
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
					return nil, cerror.ErrDeadlineExceeded
				} else if ctx.Err() == context.Canceled {
					return nil, cerror.ErrCanceled
				} else {
					return nil, cerror.ConvertStdError(ctx.Err())
				}
			}
		}
		server.lker.Lock()
		c.lker.RUnlock()
		if !server.Pickable() {
			server.lker.Unlock()
			continue
		}
		start := time.Now()
		//check timeout
		if msg.Deadline != 0 && msg.Deadline <= start.UnixNano()+int64(5*time.Millisecond) {
			server.lker.Unlock()
			return nil, cerror.ErrDeadlineExceeded
		}
		//send message
		if e := server.peer.SendMessage(ctx, d); e != nil {
			if e == stream.ErrMsgLarge {
				server.lker.Unlock()
				return nil, ERRREQMSGLARGE
			} else if e == stream.ErrConnClosed {
				server.status = 4
			}
			server.lker.Unlock()
			continue
		}
		//send message success,store req,add req num
		server.reqs[msg.Callid] = r
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		server.lker.Unlock()
		select {
		case <-r.finish:
			//req finished,delete req,reduce req num
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "RPC", path, &start, &end, r.err)
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
			var err error
			if r.err == nil {
				err = nil
			} else {
				err = r.err
			}
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
			end := time.Now()
			if ctx.Err() == context.DeadlineExceeded {
				trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "RPC", path, &start, &end, cerror.ErrDeadlineExceeded)
				return nil, cerror.ErrDeadlineExceeded
			} else if ctx.Err() == context.Canceled {
				trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "RPC", path, &start, &end, cerror.ErrCanceled)
				return nil, cerror.ErrCanceled
			} else {
				e := cerror.ConvertStdError(ctx.Err())
				trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "RPC", path, &start, &end, e)
				return nil, e
			}
		}
	}
}

type req struct {
	callid uint64
	finish chan struct{}
	resp   []byte
	err    *cerror.Error
}

func (r *req) reset() {
	r.callid = 0
	for len(r.finish) > 0 {
		<-r.finish
	}
	r.resp = nil
	r.err = nil
}
func (c *RpcClient) getreq(callid uint64) *req {
	r, ok := c.reqpool.Get().(*req)
	if ok {
		r.callid = callid
		return r
	}
	return &req{
		callid: callid,
		finish: make(chan struct{}, 1),
		resp:   nil,
		err:    nil,
	}
}
func (c *RpcClient) putreq(r *req) {
	r.reset()
	c.reqpool.Put(r)
}
