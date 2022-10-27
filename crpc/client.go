package crpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/protobuf/proto"
)

type PickHandler func(servers []*ServerForPick) *ServerForPick

// key server's addr,format: ip:port
// if the value is nil means this server node is offline
type DiscoveryHandler func(servergroup, servername string) (map[string]*RegisterData, error)
type RegisterData struct {
	//server register on which discovery server
	//if this is empty means this server node is offline
	DServers map[string]*struct{}
	Addition []byte
}

type ClientConfig struct {
	GlobalTimeout    time.Duration //global timeout for every rpc call
	ConnectTimeout   time.Duration //default 500ms
	HeartPorbe       time.Duration //default 1s,3 probe missing means disconnect
	MaxMsgLen        uint32        //default 64M,min 64k
	UseTLS           bool          //crpc or crpcs
	SkipVerifyTLS    bool          //don't verify the server's cert
	CAs              []string      //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	Discover         DiscoveryHandler //this function is used to resolve server addrs
	DiscoverInterval time.Duration    //the frequency call Discover to resolve the server addrs,default 10s,min 1s
}

func (c *ClientConfig) validate() {
	//this is checked by stream,don't need to check here
	// if c.ConnectTimeout <= 0 {
	// c.ConnectTimeout = time.Millisecond * 500
	// }
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	//this is checked by stream,don't need to check here
	// if c.HeartPorbe < time.Second {
	// c.HeartPorbe = time.Second
	// }
	if c.DiscoverInterval <= 0 {
		c.DiscoverInterval = time.Second * 10
	} else if c.DiscoverInterval < time.Second {
		c.DiscoverInterval = time.Second
	}
	//this is checked by stream,don't need to check here
	// if c.MaxMsgLen == 0 {
	// c.MaxMsgLen = 1024 * 1024 * 64
	// } else if c.MaxMsgLen < 65535 {
	// c.MaxMsgLen = 65535
	// }
}

type CrpcClient struct {
	selfappname   string
	serverappname string //group.name
	c             *ClientConfig
	tlsc          *tls.Config
	instance      *stream.Instance

	resolver *corelibResolver
	balancer *corelibBalancer

	stop    *graceful.Graceful
	reqpool *sync.Pool
}

func NewCrpcClient(c *ClientConfig, selfgroup, selfname, servergroup, servername string) (*CrpcClient, error) {
	serverappname := servergroup + "." + servername
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
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
				return nil, errors.New("[crpc.client] read cert file:" + cert + " error:" + e.Error())
			}
			if !certpool.AppendCertsFromPEM(certPEM) {
				return nil, errors.New("[crpc.client] load cert file:" + cert + " error:" + e.Error())
			}
		}
	}
	client := &CrpcClient{
		selfappname:   selfappname,
		serverappname: serverappname,
		c:             c,
		tlsc: &tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		},

		reqpool: &sync.Pool{},
		stop:    graceful.New(),
	}
	client.balancer = newCorelibBalancer(client)
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout,
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	//tcp instalce
	instancec.VerifyFunc = client.verifyfunc
	instancec.OnlineFunc = client.onlinefunc
	instancec.UserdataFunc = client.userfunc
	instancec.OfflineFunc = client.offlinefunc
	client.instance, _ = stream.NewInstance(instancec)
	//init discover
	client.resolver = newCorelibResolver(servergroup, servername, client)
	return client, nil
}

func (c *CrpcClient) ResolveNow() {
	c.resolver.ResolveNow()
}
func (c *CrpcClient) Close() {
	c.stop.Close(c.resolver.Close, c.instance.Stop)
}

func (c *CrpcClient) start(server *ServerForPick, reconnect bool) {
	if reconnect && !c.balancer.ReconnectCheck(server) {
		//can't reconnect to server
		return
	}
	var addr string
	if c.c.UseTLS {
		addr = "tcps://" + server.addr
	} else {
		addr = "tcp://" + server.addr
	}
	if !c.instance.StartClient(addr, common.Str2byte(c.serverappname), c.tlsc) {
		go c.start(server, true)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	//verify success
	return nil, true
}

func (c *CrpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.getRegisterServer(p.GetRemoteAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	server.setpeer(p)
	c.balancer.RebuildPicker(true)
	log.Info(nil, "[crpc.client.onlinefunc] server RemoteAddr:", p.GetRemoteAddr(), "online")
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[crpc.client.userfunc] server RemoteAddr:", p.GetRemoteAddr(), "data format error:", e)
		return
	}
	server.lker.Lock()
	if msg.Error != nil && cerror.Equal(msg.Error, cerror.ErrClosing) {
		//update pickable status
		server.setpeer(nil)
		//all calls' callid big and equal then this msg's callid are unprocessed
		for callid, req := range server.reqs {
			if callid >= msg.Callid {
				req.respdata = msg.Body
				req.err = msg.Error
				req.finish <- nil
				delete(server.reqs, callid)
			}
		}
	} else if req, ok := server.reqs[msg.Callid]; ok {
		req.respdata = msg.Body
		req.err = msg.Error
		req.finish <- nil
		delete(server.reqs, msg.Callid)
	}
	server.lker.Unlock()
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	log.Info(nil, "[crpc.client.offlinefunc] server RemoteAddr:", p.GetRemoteAddr(), "offline")
	server.setpeer(nil)
	c.balancer.RebuildPicker(false)
	server.lker.Lock()
	for callid, req := range server.reqs {
		req.respdata = nil
		req.err = cerror.ErrClosed
		req.finish <- nil
		delete(server.reqs, callid)
	}
	server.lker.Unlock()
	go c.start(server, true)
}

var errPickAgain = errors.New("[crpc.client] picked server closed")

var ClientClosed = errors.New("[crpc.client] closed")

func (c *CrpcClient) Call(ctx context.Context, path string, in []byte, metadata map[string]string) ([]byte, error) {
	if !c.stop.AddOne() {
		return nil, ClientClosed
	}
	defer c.stop.DoneOne()
	msg := &Msg{
		Type:     MsgType_CALL,
		Path:     path,
		Body:     in,
		Metadata: metadata,
	}
	traceid, selfappname, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid != "" {
		msg.Tracedata = map[string]string{
			"Traceid":      traceid,
			"SourceApp":    selfappname,
			"SourceMethod": selfmethod,
			"SourcePath":   selfpath,
			"Deep":         strconv.Itoa(selfdeep),
		}
	}
	if c.c.GlobalTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok {
		msg.Deadline = dl.UnixNano()
	}
	r := c.getreq(msg)
	for {
		start := time.Now()
		server, e := c.balancer.Pick(ctx)
		if e != nil {
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverappname, "pick failed,no server addr", "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.serverappname, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		if ok && dl.UnixNano() <= time.Now().UnixNano()+int64(5*time.Millisecond) {
			//at least 5ms for net lag and server logic
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverappname, server.addr, "CRPC", path, &start, &end, cerror.ErrDeadlineExceeded)
			monitor.CrpcClientMonitor(c.serverappname, "CRPC", path, cerror.ErrDeadlineExceeded, uint64(end.UnixNano()-start.UnixNano()))
			return nil, cerror.ErrDeadlineExceeded
		}
		msg.Callid = atomic.AddUint64(&server.callid, 1)
		atomic.AddInt32(&server.Pickinfo.Activecalls, 1)
		if e = server.sendmessage(ctx, r); e != nil {
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			if e == errPickAgain {
				continue
			}
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverappname, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.serverappname, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
		select {
		case <-r.finish:
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverappname, server.addr, "CRPC", path, &start, &end, r.err)
			monitor.CrpcClientMonitor(c.serverappname, "CRPC", path, r.err, uint64(end.UnixNano()-start.UnixNano()))
			if r.err != nil {
				//req error,update last fail time
				server.Pickinfo.LastFailTime = time.Now().UnixNano()
				if cerror.Equal(r.err, cerror.ErrClosing) {
					server.closing = true
					//triger discovery
					c.ResolveNow()
					//server is closing,this req can be retry
					r.respdata = nil
					r.err = nil
					continue
				}
				e = r.err
			}
			resp := r.respdata
			c.putreq(r)
			//resp and err maybe both nil
			return resp, e
		case <-ctx.Done():
			atomic.AddInt32(&server.Pickinfo.Activecalls, -1)
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
			//update last fail time
			server.Pickinfo.LastFailTime = time.Now().UnixNano()
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
			log.Trace(ctx, log.CLIENT, c.serverappname, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.serverappname, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
	}
}

type req struct {
	finish   chan *struct{}
	reqdata  *Msg
	respdata []byte
	err      *cerror.Error
}

func (c *CrpcClient) getreq(reqdata *Msg) *req {
	r, ok := c.reqpool.Get().(*req)
	if ok {
		for len(r.finish) > 0 {
			<-r.finish
		}
		r.reqdata = reqdata
		r.respdata = nil
		r.err = nil
		return r
	}
	return &req{
		finish:   make(chan *struct{}, 1),
		reqdata:  reqdata,
		respdata: nil,
		err:      nil,
	}
}
func (c *CrpcClient) putreq(r *req) {
	c.reqpool.Put(r)
}
