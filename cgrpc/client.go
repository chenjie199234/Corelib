package cgrpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/discover"
	"github.com/chenjie199234/Corelib/internal/resolver"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/graceful"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	gresolver "google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
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
	opts = append(opts, grpc.WithRecvBufferPool(pool.GetPool()))
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
	conn, e := grpc.Dial("corelib:///"+serverapp, opts...)
	if e != nil {
		return nil, e
	}
	client.conn = conn
	return client, nil
}

func (c *CGrpcClient) ResolveNow() {
	c.resolver.Now()
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
		c.resolver.Close()
		c.conn.Close()
	} else {
		c.stop.Close(c.resolver.Close, func() { c.conn.Close() })
	}
}

var ClientClosed = errors.New("[cgrpc.client] closed")

// forceaddr: most of the time this should be empty
//
//	if it is not empty,this request will try to transport to this specific addr's server
//	if this specific server doesn't exist,cerror.ErrNoSpecificServer will return
//	if the DI is static:the forceaddr can be addr in the DI's addrs list
//	if the DI is dns:the forceaddr can be addr in the dns resolve result
//	if the DI is kubernetes:the forceaddr can be addr in the endpoints
func (c *CGrpcClient) Call(ctx context.Context, path string, req interface{}, resp interface{}, metadata map[string]string, forceaddr string) error {
	if e := c.stop.Add(1); e != nil {
		if e == graceful.ErrClosing {
			return cerror.ErrClientClosing
		}
		return cerror.ErrBusy
	}
	defer c.stop.DoneOne()

	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid == "" {
		ctx = log.InitTrace(ctx, "", c.self, host.Hostip, "unknown", "unknown", 0)
		traceid, _, _, selfmethod, selfpath, selfdeep = log.GetTrace(ctx)
	}
	ctx = context.WithValue(ctx, forceaddrkey{}, forceaddr)
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout.StdDuration()))
		defer cancel()
	}
	md := gmetadata.New(nil)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		md.Set("Core-Metadata", common.Byte2str(d))
	}
	md.Set("Core-Tracedata", traceid, c.self, selfmethod, selfpath, strconv.Itoa(selfdeep))
	md.Set("Core-Target", c.server)
	ctx = gmetadata.NewOutgoingContext(ctx, md)
	for {
		start := time.Now()
		p := &peer.Peer{}
		e := transGrpcError(c.conn.Invoke(ctx, path, req, resp, grpc.Peer(p)))
		end := time.Now()
		if p.Addr == nil {
			//pick error or create stream unretryable error,req doesn't send
		} else {
			//req send,recv error
			log.Trace(ctx, log.CLIENT, c.server, p.Addr.String(), "GRPC", path, &start, &end, e)
			monitor.GrpcClientMonitor(c.server, "GRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
		}
		if cerror.Equal(e, cerror.ErrServerClosing) {
			continue
		}
		if e == nil {
			return nil
		}
		return e
	}
}

func transGrpcError(e error) *cerror.Error {
	if e == nil {
		return nil
	}
	s, _ := status.FromError(e)
	if s == nil {
		return nil
	}
	switch s.Code() {
	case codes.OK:
		return nil
	case codes.Canceled:
		return cerror.ErrCanceled
	case codes.DeadlineExceeded:
		return cerror.ErrDeadlineExceeded
	case codes.Unknown:
		return cerror.ConvertErrorstr(s.Message())
	case codes.InvalidArgument:
		return cerror.MakeError(-1, http.StatusBadRequest, s.Message())
	case codes.NotFound:
		return cerror.MakeError(-1, http.StatusNotFound, s.Message())
	case codes.AlreadyExists:
		return cerror.MakeError(-1, http.StatusBadRequest, s.Message())
	case codes.PermissionDenied:
		return cerror.MakeError(-1, http.StatusForbidden, s.Message())
	case codes.ResourceExhausted:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.FailedPrecondition:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.Aborted:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.OutOfRange:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unimplemented:
		return cerror.MakeError(-1, http.StatusNotImplemented, s.Message())
	case codes.Internal:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unavailable:
		return cerror.MakeError(-1, http.StatusServiceUnavailable, s.Message())
	case codes.DataLoss:
		return cerror.MakeError(-1, http.StatusNotFound, s.Message())
	case codes.Unauthenticated:
		return cerror.MakeError(-1, http.StatusUnauthorized, s.Message())
	default:
		ee := cerror.ConvertErrorstr(s.Message())
		ee.SetHttpcode(int32(s.Code()))
		return ee
	}
}
