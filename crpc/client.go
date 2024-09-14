package crpc

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/trace"
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

	client.balancer = newCorelibBalancer(client)
	client.resolver = resolver.NewCorelibResolver(client.balancer, client.discover, discover.Crpc)
	client.resolver.Start()
	return client, nil
}

func (c *CrpcClient) ResolveNow() {
	go c.resolver.Now()
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
	if !c.instance.StartClient(server.addr, false, common.STB(c.server), c.tlsc) {
		go c.start(server, true)
	}
}

func (c *CrpcClient) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, string, bool) {
	//verify success
	return nil, "", true
}

func (c *CrpcClient) onlinefunc(ctx context.Context, p *stream.Peer) bool {
	//online success,update success
	server := c.balancer.getRegisterServer(p.GetRawConnectAddr())
	if server == nil {
		return false
	}
	p.SetData(unsafe.Pointer(server))
	server.setpeer(p)
	server.closing = 0
	c.balancer.RebuildPicker(server.addr, true)
	slog.InfoContext(nil, "[crpc.client] online", slog.String("sname", c.server), slog.String("sip", server.addr))
	return true
}

func (c *CrpcClient) userfunc(p *stream.Peer, data []byte) {
	server := (*ServerForPick)(p.GetData())
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		//this is impossible
		slog.ErrorContext(nil, "[crpc.client] userdata format wrong", slog.String("sname", c.server), slog.String("sip", server.addr))
		return
	}
	switch msg.H.Type {
	case MsgType_CloseRead:
		if rw := server.getrw(msg.H.Callid); rw != nil {
			atomic.AndInt32(&rw.status, 0b0111)
		}
	case MsgType_CloseSend:
		if rw := server.getrw(msg.H.Callid); rw != nil {
			atomic.AndInt32(&rw.status, 0b1011)
			rw.reader.Close()
		}
	case MsgType_CloseReadSend:
		rw := server.getrw(msg.H.Callid)
		if rw != nil {
			atomic.AndInt32(&rw.status, 0b0111)
			atomic.AndInt32(&rw.status, 0b1011)
			rw.reader.Close()
		}
	case MsgType_Send:
		if msg.B.Error != nil && cerror.Equal(msg.B.Error, cerror.ErrServerClosing) {
			if atomic.SwapInt32(&server.closing, 1) == 0 {
				//set the lowest pick priority
				server.Pickinfo.SetDiscoverServerOffline(0)
				//rebuild picker
				c.balancer.RebuildPicker(server.addr, false)
				//triger discover
				c.resolver.Now()
			}
		}
		if msg.H != nil {
			if rw := server.getrw(msg.H.Callid); rw != nil {
				rw.cache(msg.B)
			}
		}
	}
}

func (c *CrpcClient) offlinefunc(p *stream.Peer) {
	server := (*ServerForPick)(p.GetData())
	slog.InfoContext(nil, "[crpc.client] offline", slog.String("sname", c.server), slog.String("sip", server.addr))
	server.setpeer(nil)
	c.balancer.RebuildPicker(server.addr, false)
	server.cleanrw()
	go c.start(server, true)
}

func (c *CrpcClient) Call(ctx context.Context, path string, in []byte) ([]byte, error) {
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return nil, cerror.ErrClientClosing
		}
		return nil, cerror.ErrBusy
	}
	defer c.stop.DoneOne()
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	var deadline int64
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UnixNano()
	}
	md := metadata.GetMetadata(ctx)
	for {
		ctx, span := trace.NewSpan(ctx, "Corelib.Crpc", trace.Client, nil)
		if span.GetParentSpanData().IsEmpty() {
			span.GetParentSpanData().SetStateKV("app", c.self)
			span.GetParentSpanData().SetStateKV("host", host.Hostip)
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		}
		span.GetSelfSpanData().SetStateKV("app", c.server)
		span.GetSelfSpanData().SetStateKV("method", "CRPC")
		span.GetSelfSpanData().SetStateKV("path", path)
		selfmethod, _ := span.GetParentSpanData().GetStateKV("method")
		selfpath, _ := span.GetParentSpanData().GetStateKV("path")
		td := map[string]string{
			"TraceID": span.GetSelfSpanData().GetTid().String(),
			"SpanID":  span.GetSelfSpanData().GetSid().String(),
			"app":     c.self,
			"host":    host.Hostip,
			"method":  selfmethod,
			"path":    selfpath,
		}
		server, done, e := c.balancer.Pick(ctx)
		if e != nil {
			return nil, e
		}
		span.GetSelfSpanData().SetStateKV("host", server.addr)
		rw := server.createrw(path, deadline, md, td)
		if e := rw.init(ctx, &MsgBody{Body: in}); e != nil {
			server.closerw(rw.callid)
			slog.ErrorContext(ctx, "[crpc.client] send call failed",
				slog.String("sname", c.server),
				slog.String("sip", server.addr),
				slog.String("path", path),
				slog.String("error", e.Error()))
			span.Finish(e)
			done(0, 0, false)
			monitor.CrpcClientMonitor(c.server, "CRPC", path, e, uint64(span.GetEnd()-span.GetStart()))
			if cerror.Equal(e, cerror.ErrClosed) {
				continue
			}
			return nil, e
		}
		out, trail, e := rw.read(ctx)
		span.Finish(e)
		var cpuusage float64
		if trail != nil {
			cpuusage, _ = strconv.ParseFloat(trail["Cpu-Usage"], 64)
		}
		done(cpuusage, uint64(span.GetEnd()-span.GetStart()), e == nil)
		monitor.CrpcClientMonitor(c.server, "CRPC", path, e, uint64(span.GetEnd()-span.GetStart()))
		if e != nil {
			if cerror.Equal(e, cerror.ErrCanceled) && trail == nil {
				//client cancel,need to tell server
				if ee := rw.cancel(); e != nil {
					slog.ErrorContext(ctx, "[crpc.client] send cancel call failed",
						slog.String("sname", c.server),
						slog.String("sip", server.addr),
						slog.String("path", path),
						slog.String("error", ee.Error()))
				}
			}
			server.closerw(rw.callid)
			if cerror.Equal(e, cerror.ErrServerClosing) {
				continue
			}
		} else {
			server.closerw(rw.callid)
		}
		return out, e
	}
}

// TODO
func (c *CrpcClient) Stream(ctx context.Context, path string) (*rw, error) {
	return nil, nil
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
