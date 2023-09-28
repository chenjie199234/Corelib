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
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/protobuf/proto"
)

type ClientConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if ctx's Deadline exist and GlobalTimeout > 0,the min(time.Now().Add(GlobalTimeout) ,ctx.Deadline()) will be used as the final deadline
	//if ctx's Deadline not exist and GlobalTimeout > 0 ,the time.Now().Add(GlobalTimeout) will be used as the final deadline
	//if ctx's deadline not exist and GlobalTimeout <=0,means no deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establich(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout,if >0 min is HeartProbe
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

type CrpcClient struct {
	self     string
	server   string
	c        *ClientConfig
	tlsc     *tls.Config
	instance *stream.Instance

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop    *graceful.Graceful
	reqpool *sync.Pool
}

// if tlsc is not nil,the tls will be actived
func NewCrpcClient(c *ClientConfig, d discover.DI, selfproject, selfgroup, selfapp, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*CrpcClient, error) {
	//pre check
	serverfullname, e := name.MakeFullName(serverproject, servergroup, serverapp)
	if e != nil {
		return nil, e
	}
	selffullname, e := name.MakeFullName(selfproject, selfgroup, selfapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ClientConfig{}
	}
	if d == nil {
		return nil, errors.New("[crpc.client] missing discover")
	}
	if !d.CheckTarget(serverfullname) {
		return nil, errors.New("[crpc.client] discover's target app not match")
	}
	client := &CrpcClient{
		self:   selffullname,
		server: serverfullname,
		c:      c,
		tlsc:   tlsc,

		discover: d,

		reqpool: &sync.Pool{},
		stop:    graceful.New(),
	}
	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Crpc)
	instancec := &stream.InstanceConfig{
		RecvIdleTimeout:    c.IdleTimeout.StdDuration(),
		HeartprobeInterval: c.HeartProbe.StdDuration(),
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout.StdDuration(),
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

// get the server's addrs from the discover.DI(the param in NewCrpcClient)
// version can be int64 or string(should only be used with == or !=)
func (c *CrpcClient) GetServerIps() (ips []string, version interface{}, lasterror error) {
	tmp, version, e := c.discover.GetAddrs(discover.NotNeed)
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
	if !c.instance.StartClient(server.addr, false, common.Str2byte(c.server), c.tlsc) {
		go c.start(server, true)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	//verify success
	return nil, true
}

func (c *CrpcClient) onlinefunc(p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.getRegisterServer(p.GetRawConnectAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	server.setpeer(p)
	c.balancer.RebuildPicker(true)
	log.Info(nil, "[crpc.client] online", log.String("sname", c.server), log.String("sip", p.GetRemoteAddr()))
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		log.Error(nil, "[crpc.client] userdata format wrong", log.String("sname", c.server), log.String("sip", p.GetRemoteAddr()))
		return
	}
	server.lker.Lock()
	if msg.Error != nil && cerror.Equal(msg.Error, cerror.ErrServerClosing) {
		//update pickable status
		server.setpeer(nil)
		server.closing = true
		//set the lowest pick priority
		server.Pickinfo.SetDiscoverServerOffline(0)
		//all calls' callid big and equal then this msg's callid are unprocessed
		for callid, req := range server.reqs {
			if callid >= msg.Callid {
				req.respmd = nil
				req.respdata = nil
				req.err = msg.Error
				req.finish <- nil
				delete(server.reqs, callid)
			}
		}
	} else if req, ok := server.reqs[msg.Callid]; ok {
		req.respmd = msg.Metadata
		req.respdata = msg.Body
		req.err = msg.Error
		req.finish <- nil
		delete(server.reqs, msg.Callid)
	}
	server.lker.Unlock()
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	log.Info(nil, "[crpc.client] offline", log.String("sname", c.server), log.String("sip", p.GetRemoteAddr()))
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

func (c *CrpcClient) Call(ctx context.Context, path string, in []byte) ([]byte, error) {
	forceaddr, _ := ctx.Value(forceaddrkey{}).(string)
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return nil, cerror.ErrClientClosing
		}
		return nil, cerror.ErrBusy
	}
	defer c.stop.DoneOne()
	msg := &Msg{
		Type:     MsgType_CALL,
		Path:     path,
		Body:     in,
		Metadata: metadata.GetMetadata(ctx),
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid == "" {
		ctx = log.InitTrace(ctx, "", c.self, host.Hostip, "unknown", "unknown", 0)
		traceid, _, _, selfmethod, selfpath, selfdeep = log.GetTrace(ctx)
	}
	msg.Tracedata = map[string]string{
		"TraceID":      traceid,
		"SourceApp":    c.self,
		"SourceMethod": selfmethod,
		"SourcePath":   selfpath,
		"Deep":         strconv.Itoa(selfdeep),
	}
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	if dl, ok := ctx.Deadline(); ok {
		msg.Deadline = dl.UnixNano()
	}
	r := c.getreq(msg)
	for {
		start := time.Now()
		server, done, e := c.balancer.Pick(ctx, forceaddr)
		if e != nil {
			return nil, e
		}
		msg.Callid = atomic.AddUint64(&server.callid, 1)
		if e = server.sendmessage(ctx, r); e != nil {
			done(0, 0, false)
			end := time.Now()
			log.Error(ctx, "[crpc.client] send message failed", log.String("sname", c.server), log.String("sip", server.addr), log.String("path", path), log.CError(e))
			log.Trace(ctx, log.CLIENT, c.server, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.server, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			if cerror.Equal(e, cerror.ErrClosed) {
				continue
			}
			return nil, e
		}
		select {
		case <-r.finish:
			end := time.Now()
			cpuusage, _ := strconv.ParseFloat(r.respmd["Cpu-Usage"], 64)
			done(cpuusage, uint64(end.UnixNano()-start.UnixNano()), r.err == nil)
			log.Trace(ctx, log.CLIENT, c.server, server.addr, "CRPC", path, &start, &end, r.err)
			monitor.CrpcClientMonitor(c.server, "CRPC", path, r.err, uint64(end.UnixNano()-start.UnixNano()))
			if r.err != nil {
				if cerror.Equal(r.err, cerror.ErrServerClosing) {
					//triger discovery
					c.resolver.Now()
					//server is closing,this req can be retry
					r.respmd = nil
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
			done(0, 0, false)
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
			log.Trace(ctx, log.CLIENT, c.server, server.addr, "CRPC", path, &start, &end, e)
			monitor.CrpcClientMonitor(c.server, "CRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
			return nil, e
		}
	}
}

type forceaddrkey struct{}

// forceaddr: most of the time this should be empty
//
//	if it is not empty,this request will try to transport to this specific addr's server
//	if this specific server doesn't exist,cerror.ErrNoSpecificServer will return
//	if the DI is static:the forceaddr can be addr in the DI's addrs list
//	if the DI is dns:the forceaddr can be addr in the dns resolve result
//	if the DI is kubernetes:the forceaddr can be addr in the endpoints
func WithForceAddr(ctx context.Context, forceaddr string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, forceaddrkey{}, forceaddr)
}

type req struct {
	finish   chan *struct{}
	reqdata  *Msg
	respdata []byte
	respmd   map[string]string
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
