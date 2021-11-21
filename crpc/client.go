package crpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

type DiscoveryHandler func(group, name string, manually <-chan *struct{}, client *CrpcClient)

type ClientConfig struct {
	ConnTimeout   time.Duration //default 500ms
	GlobalTimeout time.Duration //global timeout for every rpc call(including connection establish time)
	HeartPorbe    time.Duration //default 1s,3 probe missing means disconnect
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	UseTLS        bool     //crpc or crpcs
	SkipVerifyTLS bool     //don't verify the server's cert
	CAs           []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker        PickHandler
	Discover      DiscoveryHandler //this function will be called in goroutine in NewCrpcClient
}

func (c *ClientConfig) validate() {
	if c.ConnTimeout <= 0 {
		c.ConnTimeout = time.Millisecond * 500
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe <= 0 {
		c.HeartPorbe = time.Second
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

type CrpcClient struct {
	selfappname string
	appname     string
	c           *ClientConfig
	tlsc        *tls.Config
	instance    *stream.Instance

	resolver *corelibResolver
	balancer *corelibBalancer

	callid  uint64
	reqpool *sync.Pool
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
		return nil, errors.New("[crpc.client] missing config")
	}
	if c.Discover == nil {
		return nil, errors.New("[crpc.client] missing discover in config")
	}
	if c.Picker == nil {
		log.Warning(nil, "[crpc.client] missing picker in config,default picker will be used")
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

		callid:  0,
		reqpool: &sync.Pool{},
	}
	client.balancer = newCorelibBalancer(client)
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
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
	client.resolver = newCorelibResolver(group, name, client)
	return client, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr
func (c *CrpcClient) UpdateDiscovery(all map[string]*RegisterData) {
	c.balancer.UpdateDiscovery(all)
}

func (c *CrpcClient) start(server *ServerForPick) {
	if atomic.LoadInt32(&server.status) == 0 {
		//reconnect to server
		if !c.balancer.ReconnectCheck(server) {
			return
		}
		atomic.StoreInt32(&server.status, 1)
	}
	tempverifydata := c.appname
	var tlsc *tls.Config
	if c.c.UseTLS {
		tlsc = c.tlsc
	}
	if !c.instance.StartTcpClient(server.addr, common.Str2byte(tempverifydata), tlsc) {
		atomic.StoreInt32(&server.status, 0)
		time.Sleep(time.Millisecond * 100)
		go c.start(server)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	//verify success
	return nil, true
}

func (c *CrpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.GetRegisterServer(p.GetRemoteAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&server.peer)), unsafe.Pointer(p))
	atomic.StoreInt32(&server.status, 2)
	c.resolver.wakemanual()
	log.Info(nil, "[crpc.client.onlinefunc] server:", p.GetPeerUniqueName(), "online")
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[crpc.client.userfunc] server:", p.GetPeerUniqueName(), "data format error:", e)
		return
	}
	if msg.Error != nil && cerror.Equal(msg.Error, errClosing) {
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
	log.Info(nil, "[crpc.client.offlinefunc] server:", p.GetPeerUniqueName(), "offline")
	atomic.StoreInt32(&server.status, 0)
	server.lker.Lock()
	for callid, req := range server.reqs {
		req.resp = nil
		req.err = ErrClosed
		req.finish <- nil
		delete(server.reqs, callid)
	}
	server.lker.Unlock()
	go c.start(server)
}

var errPickAgain = errors.New("[crpc.client] picked server closed")

func (c *CrpcClient) Call(ctx context.Context, functimeout time.Duration, path string, in []byte, metadata map[string]string) ([]byte, error) {
	start := time.Now()
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
	if ok && dl.UnixNano() <= start.UnixNano()+int64(5*time.Millisecond) {
		return nil, cerror.ErrDeadlineExceeded
	}
	msg := &Msg{
		Callid:   atomic.AddUint64(&c.callid, 1),
		Type:     MsgType_CALL,
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
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			return nil, e
		}
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
				if cerror.Equal(r.err, errClosing) {
					//triger manually discovery
					c.resolver.manual(nil)
					//server is closing,this req can be retry
					r.resp = nil
					r.err = nil
					continue
				}
				e = r.err
			}
			resp := r.resp
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
				canceldata, _ := proto.Marshal(&Msg{
					Callid: msg.Callid,
					Type:   MsgType_CANCEL,
				})
				go server.sendcancel(context.Background(), canceldata)
			} else {
				//this is impossible
				e = cerror.ConvertStdError(ctx.Err())
			}
			end := time.Now()
			trace.Trace(ctx, trace.CLIENT, c.appname, server.addr, "CRPC", path, &start, &end, e)
			return nil, e
		}
	}
}

type req struct {
	callid uint64
	finish chan *struct{}
	req    []byte
	resp   []byte
	err    *cerror.Error
}

func (c *CrpcClient) getreq(callid uint64, reqdata []byte) *req {
	r, ok := c.reqpool.Get().(*req)
	if ok {
		r.callid = callid
		for len(r.finish) > 0 {
			<-r.finish
		}
		r.req = reqdata
		r.resp = nil
		r.err = nil
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
	c.reqpool.Put(r)
}
