package crpc

import (
	"context"
	"crypto/tls"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/protobuf/proto"
)

type ClientConfig struct {
	GlobalTimeout  time.Duration //global timeout for every rpc call,<=0 means no timeout
	ConnectTimeout time.Duration //default 500ms
	HeartProbe     time.Duration //default 1s,3 probe missing means disconnect
	MaxMsgLen      uint32        //default 64M,min 64k
}

type CrpcClient struct {
	selfapp   string
	serverapp string //group.name
	c         *ClientConfig
	tlsc      *tls.Config
	instance  *stream.Instance

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop    *graceful.Graceful
	reqpool *sync.Pool
}

// if tlsc is not nil,the tls will be actived
func NewCrpcClient(c *ClientConfig, d discover.DI, selfappgroup, selfappname, serverappgroup, serverappname string, tlsc *tls.Config) (*CrpcClient, error) {
	serverapp := serverappgroup + "." + serverappname
	selfapp := selfappgroup + "." + selfappname
	if e := name.FullCheck(selfapp); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ClientConfig{}
	}
	if d == nil {
		return nil, errors.New("[crpc.client] missing discover")
	}
	if !d.CheckApp(serverapp) {
		return nil, errors.New("[crpc.client] discover's target app not match")
	}
	client := &CrpcClient{
		selfapp:   selfapp,
		serverapp: serverapp,
		c:         c,
		tlsc:      tlsc,

		discover: d,

		reqpool: &sync.Pool{},
		stop:    graceful.New(),
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Crpc)
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartProbe,
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
	return client, nil
}

func (c *CrpcClient) ResolveNow() {
	c.resolver.Now()
}

func (c *CrpcClient) GetServerIps() (ips []string, lasterror error) {
	tmp, e := c.discover.GetAddrs(discover.NotNeed)
	ips = make([]string, 0, len(tmp))
	for k := range tmp {
		ips = append(ips, k)
	}
	lasterror = e
	return
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (c *CrpcClient) Close(force bool) {
	if force {
		c.resolver.Close()
		c.instance.Stop()
	} else {
		c.stop.Close(c.resolver.Close, c.instance.Stop)
	}
}

func (c *CrpcClient) start(server *ServerForPick, reconnect bool) {
	if reconnect && !c.balancer.ReconnectCheck(server) {
		//can't reconnect to server
		return
	}
	var addr string
	if c.tlsc != nil {
		addr = "tcps://" + server.addr
	} else {
		addr = "tcp://" + server.addr
	}
	if !c.instance.StartClient(addr, common.Str2byte(c.serverapp), c.tlsc) {
		go c.start(server, true)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	//verify success
	return nil, true
}

func (c *CrpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.getRegisterServer(p.GetRawAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	server.setpeer(p)
	c.balancer.RebuildPicker(true)
	log.Info(nil, "[crpc.client.onlinefunc] server:", c.serverapp+":"+p.GetRemoteAddr(), "online")
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[crpc.client.userfunc] server:", c.serverapp+":"+p.GetRemoteAddr(), "data format wrong:", e)
		return
	}
	server.lker.Lock()
	if msg.Error != nil && cerror.Equal(msg.Error, cerror.ErrServerClosing) {
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
	log.Info(nil, "[crpc.client.offlinefunc] server:", c.serverapp+":"+p.GetRemoteAddr(), "offline")
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

func (c *CrpcClient) Call(ctx context.Context, path string, in []byte, metadata map[string]string) ([]byte, error) {
	if !c.stop.AddOne() {
		return nil, cerror.ErrClientClosing
	}
	defer c.stop.DoneOne()
	msg := &Msg{
		Type:     MsgType_CALL,
		Path:     path,
		Body:     in,
		Metadata: metadata,
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid == "" {
		ctx = log.InitTrace(ctx, "", c.selfapp, host.Hostip, "unknown", "unknown", 0)
		traceid, _, _, selfmethod, selfpath, selfdeep = log.GetTrace(ctx)
	}
	msg.Tracedata = map[string]string{
		"TraceID":      traceid,
		"SourceApp":    c.selfapp,
		"SourceMethod": selfmethod,
		"SourcePath":   selfpath,
		"Deep":         strconv.Itoa(selfdeep),
	}
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	if dl, ok := ctx.Deadline(); ok {
		msg.Deadline = dl.UnixNano()
	}
	r := c.getreq(msg)
	for {
		start := time.Now()
		server, done, e := c.balancer.Pick(ctx)
		if e != nil {
			return nil, e
		}
		msg.Callid = atomic.AddUint64(&server.callid, 1)
		if e = server.sendmessage(ctx, r); e != nil {
			done()
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverapp, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.serverapp, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			if cerror.Equal(e, cerror.ErrClosed) {
				continue
			}
			return nil, e
		}
		select {
		case <-r.finish:
			done()
			end := time.Now()
			log.Trace(ctx, log.CLIENT, c.serverapp, server.addr, "CRPC", path, &start, &end, r.err)
			monitor.CrpcClientMonitor(c.serverapp, "CRPC", path, r.err, uint64(end.UnixNano()-start.UnixNano()))
			if r.err != nil {
				if cerror.Equal(r.err, cerror.ErrServerClosing) {
					server.closing = true
					//triger discovery
					c.resolver.Now()
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
			done()
			server.lker.Lock()
			delete(server.reqs, msg.Callid)
			server.lker.Unlock()
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
			log.Trace(ctx, log.CLIENT, c.serverapp, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.serverapp, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
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
