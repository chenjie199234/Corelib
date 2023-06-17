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
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
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
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

type PickHandler func(servers []*ServerForPick) *ServerForPick

type ClientConfig struct {
	GlobalTimeout  time.Duration //global timeout for every rpc call,<=0 means no timeout
	ConnectTimeout time.Duration //default 500ms
	HeartProbe     time.Duration //default 10s,min 10s
	MaxMsgLen      uint32        //default 64M,min 64k
}

func (c *ClientConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = time.Millisecond * 500
	}
	if c.HeartProbe < time.Second*10 {
		c.HeartProbe = time.Second * 10
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
}

type CGrpcClient struct {
	selfapp   string
	serverapp string //group.name
	c         *ClientConfig
	tlsc      *tls.Config
	conn      *grpc.ClientConn

	resolver *corelibResolver
	balancer *corelibBalancer
	picker   PickHandler
	discover discover.DI

	stop *graceful.Graceful
}

// if tlsc is not nil,the tls will be actived
func NewCGrpcClient(c *ClientConfig, p PickHandler, d discover.DI, selfappgroup, selfappname, serverappgroup, serverappname string, tlsc *tls.Config) (*CGrpcClient, error) {
	serverapp := serverappgroup + "." + serverappname
	selfapp := selfappgroup + "." + selfappname
	if e := name.FullCheck(selfapp); e != nil {
		return nil, e
	}
	if c == nil {
		return nil, errors.New("[cgrpc.client] missing config")
	}
	if d == nil {
		return nil, errors.New("[cgrpc.client] missing discover in config")
	}
	if !d.CheckApp(serverapp) {
		return nil, errors.New("[cgrpc.client] discover's target app not match")
	}
	if p == nil {
		log.Warning(nil, "[cgrpc.client] missing picker in config,default picker will be used")
		p = defaultPicker
	}
	c.validate()
	client := &CGrpcClient{
		selfapp:   selfapp,
		serverapp: serverapp,
		c:         c,
		tlsc:      tlsc,
		picker:    p,
		discover:  d,
		stop:      graceful.New(),
	}
	opts := make([]grpc.DialOption, 0, 10)
	opts = append(opts, grpc.WithDisableRetry())
	if tlsc == nil {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(client.tlsc)))
	}
	opts = append(opts, grpc.WithConnectParams(grpc.ConnectParams{
		MinConnectTimeout: c.ConnectTimeout,
		Backoff: backoff.Config{
			BaseDelay: time.Millisecond * 100,
			MaxDelay:  time.Millisecond * 100,
		}, //reconnect immediately when disconnect,reconnect delay 100ms when connect failed
	}))
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: c.HeartProbe, Timeout: c.HeartProbe*3 + c.HeartProbe/3}))
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(c.MaxMsgLen))))
	//balancer
	balancer.Register(&balancerBuilder{c: client})
	opts = append(opts, grpc.WithDisableServiceConfig())
	opts = append(opts, grpc.WithDefaultServiceConfig("{\"loadBalancingConfig\":[{\"corelib\":{}}]}"))
	//resolver
	opts = append(opts, grpc.WithResolvers(&resolverBuilder{c: client}))
	conn, e := grpc.Dial("corelib:///"+serverapp, opts...)
	if e != nil {
		return nil, e
	}
	client.conn = conn
	return client, nil
}

func (c *CGrpcClient) ResolveNow() {
	c.resolver.ResolveNow(resolver.ResolveNowOptions{})
}

func (c *CGrpcClient) GetServerIps() (ips []string, lasterror error) {
	tmp, e := c.discover.GetAddrs(discover.NotNeed)
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

func (c *CGrpcClient) Call(ctx context.Context, path string, req interface{}, resp interface{}, metadata map[string]string) error {
	if !c.stop.AddOne() {
		return ClientClosed
	}
	defer c.stop.DoneOne()
	if c.c.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	md := gmetadata.New(nil)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		md.Set("Core-Metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid == "" {
		ctx = log.InitTrace(ctx, "", c.selfapp, host.Hostip, "unknown", "unknown", 0)
		traceid, _, _, selfmethod, selfpath, selfdeep = log.GetTrace(ctx)
	}
	md.Set("Core-Tracedata", traceid, c.selfapp, selfmethod, selfpath, strconv.Itoa(selfdeep))
	md.Set("Core-Target", c.serverapp)
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
			log.Trace(ctx, log.CLIENT, c.serverapp, p.Addr.String(), "GRPC", path, &start, &end, e)
			monitor.GrpcClientMonitor(c.serverapp, "GRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
		}
		if cerror.Equal(e, cerror.ErrClosing) {
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
		return cerror.MakeError(-1, http.StatusServiceUnavailable, s.Message())
	default:
		ee := cerror.ConvertErrorstr(s.Message())
		ee.SetHttpcode(int32(s.Code()))
		return ee
	}
}
