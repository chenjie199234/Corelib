package cgrpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	cmetadata "github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/experimental"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	gresolver "google.golang.org/grpc/resolver"
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
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

func (c *ClientConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(time.Second * 3)
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
	}
	if c.HeartProbe <= 0 {
		c.HeartProbe = ctime.Duration(time.Second * 5)
	} else if c.HeartProbe.StdDuration() < time.Second {
		c.HeartProbe = ctime.Duration(time.Second)
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
}

type CGrpcClient struct {
	self   string
	server string
	c      *ClientConfig
	tlsc   *tls.Config
	conn   *grpc.ClientConn

	resolver *resolver.CorelibResolver
	balancer *corelibBalancer
	discover discover.DI

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be actived
func NewCGrpcClient(c *ClientConfig, d discover.DI, selfproject, selfgroup, selfapp, serverproject, servergroup, serverapp string, tlsc *tls.Config) (*CGrpcClient, error) {
	if tlsc != nil {
		tlsc = tlsc.Clone()
	}
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
	c.validate()
	if d == nil {
		return nil, errors.New("[cgrpc.client] missing discover in config")
	}
	if !d.CheckTarget(serverfullname) {
		return nil, errors.New("[cgrpc.client] discover's target app not match")
	}
	client := &CGrpcClient{
		self:     selffullname,
		server:   serverfullname,
		c:        c,
		tlsc:     tlsc,
		discover: d,
		stop:     graceful.New(),
	}
	opts := make([]grpc.DialOption, 0, 10)
	opts = append(opts, experimental.WithBufferPool(bpool.GetGrpcPool()))
	opts = append(opts, grpc.WithDisableRetry())
	if tlsc == nil {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(client.tlsc)))
	}
	opts = append(opts, grpc.WithConnectParams(grpc.ConnectParams{
		MinConnectTimeout: c.ConnectTimeout.StdDuration(),
		Backoff: backoff.Config{
			BaseDelay: time.Millisecond * 100,
			MaxDelay:  time.Millisecond * 100,
		}, //reconnect immediately when disconnect,reconnect delay 100ms when connect failed
	}))
	if c.IdleTimeout > 0 {
		opts = append(opts, grpc.WithIdleTimeout(c.IdleTimeout.StdDuration()))
	}
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: c.HeartProbe.StdDuration(), Timeout: c.HeartProbe.StdDuration() * 3, PermitWithoutStream: true}))
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(c.MaxMsgLen))))
	//balancer
	balancer.Register(&balancerBuilder{c: client})
	opts = append(opts, grpc.WithDisableServiceConfig())
	opts = append(opts, grpc.WithDefaultServiceConfig("{\"loadBalancingConfig\":[{\"corelib\":{}}]}"))
	//resolver
	gresolver.Register(&resolverBuilder{c: client})
	client.conn, e = grpc.NewClient("corelib:///"+serverapp, opts...)
	if e != nil {
		return nil, e
	}
	return client, nil
}

func (c *CGrpcClient) ResolveNow() {
	go c.resolver.Now()
}

// get the server's addrs from the discover.DI(the param in NewCGrpcClient)
// version can be int64 or string(should only be used with == or !=)
func (c *CGrpcClient) GetServerIps() (ips []string, version interface{}, lasterror error) {
	tmp, version, e := c.discover.GetAddrs(discover.NotNeed)
	ips = make([]string, 0, len(tmp))
	for k := range tmp {
		ips = append(ips, k)
	}
	lasterror = e
	return
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (c *CGrpcClient) Close(force bool) {
	if force {
		c.conn.Close()
	} else {
		c.stop.Close(nil, func() { c.conn.Close() })
	}
}

var ClientClosed = errors.New("[cgrpc.client] closed")

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

func (c *CGrpcClient) Invoke(ctx context.Context, path string, req, reply any, opts ...grpc.CallOption) error {
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	md := cmetadata.GetMetadata(ctx)
	gmd := gmetadata.New(nil)
	gmd.Set("Core-Target", c.server)
	if len(md) > 0 {
		d, _ := json.Marshal(md)
		gmd.Set("Core-Metadata", common.BTS(d))
	}
	for {
		ctx, span := trace.NewSpan(ctx, "Corelib.CGrpc", trace.Client, nil)
		if span.GetParentSpanData().IsEmpty() {
			span.GetParentSpanData().SetStateKV("app", c.self)
			span.GetParentSpanData().SetStateKV("host", host.Hostip)
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		}
		span.GetSelfSpanData().SetStateKV("app", c.server)
		span.GetSelfSpanData().SetStateKV("method", "GRPC")
		span.GetSelfSpanData().SetStateKV("path", path)
		gmd.Set("traceparent", span.GetSelfSpanData().FormatTraceParent())
		gmd.Set("tracestate", span.GetParentSpanData().FormatTraceState())
		e := transGrpcError(c.conn.Invoke(gmetadata.NewOutgoingContext(ctx, gmd), path, req, reply), true)
		if cerror.Equal(e, cerror.ErrServerClosing) || cerror.Equal(e, cerror.ErrTarget) {
			continue
		}
		//fix the interface nil problem
		if e == nil {
			return nil
		}
		return e
	}
}

func (c *CGrpcClient) NewStream(ctx context.Context, desc *grpc.StreamDesc, path string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	md := cmetadata.GetMetadata(ctx)
	gmd := gmetadata.New(nil)
	gmd.Set("Core-Target", c.server)
	if len(md) > 0 {
		d, _ := json.Marshal(md)
		gmd.Set("Core-Metadata", common.BTS(d))
	}
	for {
		ctx, span := trace.NewSpan(ctx, "Corelib.CGrpc", trace.Client, nil)
		if span.GetParentSpanData().IsEmpty() {
			span.GetParentSpanData().SetStateKV("app", c.self)
			span.GetParentSpanData().SetStateKV("host", host.Hostip)
			span.GetParentSpanData().SetStateKV("method", "unknown")
			span.GetParentSpanData().SetStateKV("path", "unknown")
		}
		span.GetSelfSpanData().SetStateKV("app", c.server)
		span.GetSelfSpanData().SetStateKV("method", "GRPC")
		span.GetSelfSpanData().SetStateKV("path", path)
		gmd.Set("traceparent", span.GetSelfSpanData().FormatTraceParent())
		gmd.Set("tracestate", span.GetParentSpanData().FormatTraceState())
		stream, e := c.conn.NewStream(ctx, desc, path, opts...)
		ee := transGrpcError(e, true)
		if cerror.Equal(ee, cerror.ErrServerClosing) || cerror.Equal(ee, cerror.ErrTarget) {
			continue
		}
		//fix the interface nil problem
		if ee != nil {
			return nil, ee
		}
		return &ClientStreamWraper{c: stream}, nil
	}
}

type ClientStreamWraper struct {
	c grpc.ClientStream
}

func (cs *ClientStreamWraper) Header() (gmetadata.MD, error) {
	md, e := cs.c.Header()
	e = transGrpcError(e, true)
	return md, e
}
func (cs *ClientStreamWraper) Trailer() gmetadata.MD {
	return cs.c.Trailer()
}
func (cs *ClientStreamWraper) CloseSend() error {
	return transGrpcError(cs.c.CloseSend(), true)
}
func (cs *ClientStreamWraper) Context() context.Context {
	return cs.c.Context()
}
func (cs *ClientStreamWraper) SendMsg(m any) error {
	return transGrpcError(cs.c.SendMsg(m), true)
}
func (cs *ClientStreamWraper) RecvMsg(m any) error {
	return transGrpcError(cs.c.RecvMsg(m), true)
}
