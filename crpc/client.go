package crpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"os"
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
type DiscoveryHandler func(group, name string, manually <-chan *struct{}, client *CrpcClient)

type ClientConfig struct {
	ConnTimeout      time.Duration
	GlobalTimeout    time.Duration //global timeout for every rpc call
	HeartPorbe       time.Duration
	GroupNum         uint32
	SocketRBuf       uint32
	SocketWBuf       uint32
	MaxMsgLen        uint32
	VerifyData       string
	UseTLS           bool     //crpc or crpcs
	SkipVerifyTLS    bool     //don't verify the server's cert
	CAs              []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	DiscoverFunction DiscoveryHandler //this function will be called in goroutine in NewRpcClient
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
type CrpcClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	tlsc        *tls.Config
	instance    *stream.Instance

	slker      *sync.RWMutex
	serversRaw []byte
	servers    map[string]*ServerForPick //key server addr

	manually     chan *struct{}
	manualNotice map[chan *struct{}]*struct{}
	mlker        *sync.Mutex

	callid  uint64
	reqpool *sync.Pool
}

type ServerForPick struct {
	addr       string
	dservers   map[string]struct{} //this app registered on which discovery server
	peer       *stream.Peer
	status     int32 //1 - working,0 - closed
	dispatcher chan *struct{}

	//active calls
	lker *sync.Mutex
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
	return atomic.LoadInt32(&s.status) == 1
}
func (s *ServerForPick) getDispatcher(ctx context.Context) error {
	if atomic.LoadInt32(&s.status) == 0 {
		return errPickAgain
	}
	if dl, ok := ctx.Deadline(); ok {
		//default we need 5ms in internet transport and server business logic
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl.Add(-time.Millisecond*5))
		defer cancel()
	}
	select {
	case _, ok := <-s.dispatcher:
		if !ok {
			return errPickAgain
		} else if atomic.LoadInt32(&s.status) == 0 {
			//double check
			close(s.dispatcher)
			return errPickAgain
		}
		return nil
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return cerror.ErrDeadlineExceeded
		} else if ctx.Err() == context.Canceled {
			return cerror.ErrCanceled
		} else {
			return cerror.ConvertStdError(ctx.Err())
		}
	}
}
func (s *ServerForPick) putDispatcher() {
	if atomic.LoadInt32(&s.status) == 1 {
		s.dispatcher <- nil
	} else {
		close(s.dispatcher)
	}
}
func (s *ServerForPick) sendmessage(ctx context.Context, r *req) (e error) {
	e = s.getDispatcher(ctx)
	if e != nil {
		return
	}
	defer s.putDispatcher()
	s.lker.Lock()
	//send message
	if e = s.peer.SendMessage(ctx, r.req); e != nil {
		s.lker.Unlock()
		if e == stream.ErrMsgLarge {
			e = ERRREQMSGLARGE
		} else if e == stream.ErrConnClosed {
			e = errPickAgain
		} else if e == context.DeadlineExceeded {
			e = cerror.ErrDeadlineExceeded
		} else if e == context.Canceled {
			e = cerror.ErrCanceled
		} else {
			e = cerror.ConvertStdError(e)
		}
		return
	}
	//send message success,store req,add req num
	s.reqs[r.callid] = r
	s.lker.Unlock()
	return
}

func NewCrpcClient(c *ClientConfig, selfgroup, selfname, group, name string) (*CrpcClient, error) {
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
	client := &CrpcClient{
		selfappname: selfappname,
		appname:     appname,
		c:           c,
		tlsc: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},

		slker:   &sync.RWMutex{},
		servers: make(map[string]*ServerForPick, 10),

		manually:     make(chan *struct{}, 1),
		manualNotice: make(map[chan *struct{}]*struct{}, 100),
		mlker:        &sync.Mutex{},

		callid:  0,
		reqpool: &sync.Pool{},
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
	go c.DiscoverFunction(group, name, client.manually, client)
	client.waitmanual(context.Background())
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

func (c *CrpcClient) manual() {
	select {
	case c.manually <- nil:
	default:
	}
}
func (c *CrpcClient) waitmanual(ctx context.Context) error {
	notice := make(chan *struct{}, 1)
	c.mlker.Lock()
	c.manualNotice[notice] = nil
	c.manual()
	c.mlker.Unlock()
	select {
	case <-notice:
		return nil
	case <-ctx.Done():
		c.mlker.Lock()
		delete(c.manualNotice, notice)
		c.mlker.Unlock()
		return ctx.Err()
	}
}
func (c *CrpcClient) wakemanual() {
	c.mlker.Lock()
	for notice := range c.manualNotice {
		notice <- nil
		delete(c.manualNotice, notice)
	}
	c.mlker.Unlock()
}

//all: key server's addr
func (c *CrpcClient) UpdateDiscovery(all map[string]*RegisterData) {
	d, _ := json.Marshal(all)
	c.slker.Lock()
	if bytes.Equal(c.serversRaw, d) {
		c.slker.Unlock()
		c.wakemanual()
		return
	}
	c.serversRaw = d
	//offline app
	for _, server := range c.servers {
		if _, ok := all[server.addr]; !ok {
			//this app unregistered
			server.dservers = nil
			server.Pickinfo.DServerNum = 0
			server.Pickinfo.DServerOffline = time.Now().UnixNano()
		}
	}
	//online app or update app's dservers
	for addr, registerdata := range all {
		server, ok := c.servers[addr]
		if !ok {
			//this is a new register
			server := &ServerForPick{
				addr:       addr,
				dservers:   registerdata.DServers,
				peer:       nil,
				status:     1,
				dispatcher: make(chan *struct{}, 1),
				lker:       &sync.Mutex{},
				reqs:       make(map[uint64]*req, 10),
				Pickinfo: &pickinfo{
					Lastfail:       0,
					Activecalls:    0,
					DServerNum:     int32(len(registerdata.DServers)),
					DServerOffline: 0,
					Addition:       registerdata.Addition,
				},
			}
			c.servers[addr] = server
			go c.start(server)
		} else {
			//this is not a new register
			//unregister on which discovery server
			for dserver := range server.dservers {
				if _, ok := registerdata.DServers[dserver]; !ok {
					server.Pickinfo.DServerOffline = time.Now().UnixNano()
					break
				}
			}
			//register on which new discovery server
			for dserver := range registerdata.DServers {
				if _, ok := server.dservers[dserver]; !ok {
					server.Pickinfo.DServerOffline = 0
					break
				}
			}
			server.dservers = registerdata.DServers
			server.Pickinfo.Addition = registerdata.Addition
			server.Pickinfo.DServerNum = int32(len(registerdata.DServers))
		}
	}
	c.slker.Unlock()
	c.wakemanual()
}
func (c *CrpcClient) start(server *ServerForPick) {
	if atomic.LoadInt32(&server.status) == 0 {
		//reconnect to server
		c.slker.Lock()
		if len(server.dservers) == 0 {
			//server already unregister,remove server
			delete(c.servers, server.addr)
			c.slker.Unlock()
			return
		}
		c.slker.Unlock()
		//need to check server register status
		c.waitmanual(context.Background())
		c.slker.Lock()
		if len(server.dservers) == 0 {
			//server already unregister,remove server
			delete(c.servers, server.addr)
			c.slker.Unlock()
			return
		}
		c.slker.Unlock()
		server.dispatcher = make(chan *struct{}, 1)
		atomic.StoreInt32(&server.status, 1)
	}
	tempverifydata := c.c.VerifyData + "|" + c.appname
	var tlsc *tls.Config
	if c.c.UseTLS {
		tlsc = c.tlsc
	}
	if !c.instance.StartTcpClient(server.addr, common.Str2byte(tempverifydata), tlsc) {
		server.lker.Lock()
		atomic.StoreInt32(&server.status, 0)
		close(server.dispatcher)
		server.lker.Unlock()
		time.Sleep(time.Millisecond * 100)
		go c.start(server)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	if common.Byte2str(peerVerifyData) != c.c.VerifyData {
		return nil, false
	}
	//verify success
	return nil, true
}
func (c *CrpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	c.slker.RLock()
	addr := p.GetRemoteAddr()
	server, ok := c.servers[addr]
	if !ok {
		//this is impossible
		c.slker.RUnlock()
		return false
	}
	if len(server.dservers) == 0 {
		//server already unregister
		c.slker.RUnlock()
		return false
	}
	c.slker.RUnlock()
	server.peer = p
	p.SetData(unsafe.Pointer(server))
	server.dispatcher <- nil
	log.Info(nil, "[rpc.client.onlinefunc] server:", p.GetPeerUniqueName(), "online")
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[rpc.client.userfunc] server:", p.GetPeerUniqueName(), "data format error:", e)
		return
	}
	if msg.Error != nil && msg.Error.Code == ERRCLOSING.Code {
		atomic.StoreInt32(&server.status, 0)
	}
	server.lker.Lock()
	req, ok := server.reqs[msg.Callid]
	if !ok {
		server.lker.Unlock()
		return
	}
	delete(server.reqs, msg.Callid)
	req.resp = msg.Body
	req.err = msg.Error
	req.finish <- nil
	server.lker.Unlock()
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	log.Info(nil, "[rpc.client.offlinefunc] server:", p.GetPeerUniqueName(), "offline")
	atomic.StoreInt32(&server.status, 0)
	server.lker.Lock()
	for callid, req := range server.reqs {
		req.resp = nil
		req.err = ERRCLOSED
		req.finish <- nil
		delete(server.reqs, callid)
	}
	server.lker.Unlock()
	go c.start(server)
}

var errPickAgain = errors.New("[rpc.client] picked server closed")

func (c *CrpcClient) Call(ctx context.Context, functimeout time.Duration, path string, in []byte, metadata map[string]string) ([]byte, error) {
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
	r := c.getreq(msg.Callid, d)
	for {
		server, e := c.pick(ctx)
		if e != nil {
			return nil, e
		}
		start := time.Now()
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		if e = server.sendmessage(ctx, r); e != nil {
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			if e == errPickAgain {
				continue
			}
			return nil, e
		}
		select {
		case <-r.finish:
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "CRPC", path, &start, &end, r.err)
			if r.err != nil {
				//req error,update last fail time
				server.Pickinfo.Lastfail = time.Now().UnixNano()
				if r.err.Code == ERRCLOSING.Code {
					//triger manually discovery
					c.manual()
					//server is closing,this req can be retry
					r.resp = nil
					r.err = nil
					continue
				}
			}
			resp := r.resp
			if r.err == nil {
				e = nil
			} else {
				e = r.err
			}
			c.putreq(r)
			//resp and err maybe both nil
			return resp, e
		case <-ctx.Done():
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			//update last fail time
			server.Pickinfo.Lastfail = time.Now().UnixNano()
			c.putreq(r)
			if ctx.Err() == context.DeadlineExceeded {
				e = cerror.ErrDeadlineExceeded
			} else if ctx.Err() == context.Canceled {
				e = cerror.ErrCanceled
			} else {
				e = cerror.ConvertStdError(ctx.Err())
			}
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "CRPC", path, &start, &end, e)
			return nil, e
		}
	}
}
func (c *CrpcClient) pick(ctx context.Context) (*ServerForPick, error) {
	refresh := false
	for {
		c.slker.RLock()
		server := c.c.Picker(c.servers)
		if server != nil {
			c.slker.RUnlock()
			return server, nil
		}
		if refresh {
			c.slker.RUnlock()
			return nil, ERRNOSERVER
		}
		c.slker.RUnlock()
		if e := c.waitmanual(ctx); e != nil {
			if e == context.DeadlineExceeded {
				return nil, cerror.ErrDeadlineExceeded
			} else if e == context.Canceled {
				return nil, cerror.ErrCanceled
			} else {
				return nil, cerror.ConvertStdError(e)
			}
		}
		refresh = true
	}
}

type req struct {
	callid uint64
	finish chan *struct{}
	req    []byte
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
func (c *CrpcClient) getreq(callid uint64, reqdata []byte) *req {
	r, ok := c.reqpool.Get().(*req)
	if ok {
		r.callid = callid
		r.req = reqdata
		return r
	}
	return &req{
		callid: callid,
		finish: make(chan *struct{}, 1),
		req:    reqdata,
		resp:   nil,
		err:    nil,
	}
}
func (c *CrpcClient) putreq(r *req) {
	r.reset()
	c.reqpool.Put(r)
}
