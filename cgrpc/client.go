package cgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

type PickHandler func(servers []*ServerForPick) *ServerForPick

type DiscoveryHandler func(servergroup, servername string, manually <-chan *struct{}, client *CGrpcClient)

type ClientConfig struct {
	ConnTimeout   time.Duration
	GlobalTimeout time.Duration //global timeout for every rpc call(including connection establish time)
	HeartPorbe    time.Duration
	SocketRBuf    uint32
	SocketWBuf    uint32
	MaxMsgLen     uint32
	UseTLS        bool             //grpc or grpcs
	SkipVerifyTLS bool             //don't verify the server's cert
	CAs           []string         //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Discover      DiscoveryHandler //this function will be called in goroutine in NewGrpcClient
	Picker        PickHandler
}

func (c *ClientConfig) validate() {
	if c.ConnTimeout <= 0 {
		c.ConnTimeout = time.Millisecond * 500
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe < time.Second {
		c.HeartPorbe = 1500 * time.Millisecond
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

type CGrpcClient struct {
	c             *ClientConfig
	selfappname   string
	serverappname string //group.name
	conn          *grpc.ClientConn
	resolver      *corelibResolver
	balancer      *corelibBalancer
}

func NewCGrpcClient(c *ClientConfig, selfgroup, selfname, servergroup, servername string) (*CGrpcClient, error) {
	serverappname := servergroup + "." + servername
	if e := name.FullCheck(serverappname); e != nil {
		return nil, e
	}
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		return nil, errors.New("[cgrpc.client] missing config")
	}
	if c.Discover == nil {
		return nil, errors.New("[cgrpc.client] missing discover in config")
	}
	if c.Picker == nil {
		log.Warning(nil, "[cgrpc.client] missing picker in config,default picker will be used")
		c.Picker = defaultPicker
	}
	c.validate()
	clientinstance := &CGrpcClient{
		c:             c,
		selfappname:   selfappname,
		serverappname: serverappname,
	}
	opts := make([]grpc.DialOption, 0)
	opts = append(opts, grpc.WithDisableRetry())
	if !c.UseTLS {
		opts = append(opts, grpc.WithInsecure())
	} else {
		var certpool *x509.CertPool
		if len(c.CAs) != 0 {
			certpool = x509.NewCertPool()
			for _, cert := range c.CAs {
				certPEM, e := os.ReadFile(cert)
				if e != nil {
					return nil, errors.New("[cgrpc.client] read cert file:" + cert + " error:" + e.Error())
				}
				if !certpool.AppendCertsFromPEM(certPEM) {
					return nil, errors.New("[cgrpc.client] load cert file:" + cert + " error:" + e.Error())
				}
			}
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: c.SkipVerifyTLS,
			RootCAs:            certpool,
		})))
	}
	opts = append(opts, grpc.WithReadBufferSize(int(c.SocketRBuf)))
	opts = append(opts, grpc.WithWriteBufferSize(int(c.SocketWBuf)))
	opts = append(opts, grpc.WithConnectParams(grpc.ConnectParams{
		MinConnectTimeout: c.ConnTimeout,
		Backoff: backoff.Config{
			BaseDelay: time.Millisecond * 100,
			MaxDelay:  time.Millisecond * 100,
		}, //reconnect immediately when disconnect,reconnect delay 100ms when connect failed
	}))
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: c.HeartPorbe, Timeout: c.HeartPorbe*3 + c.HeartPorbe/3}))
	opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(c.MaxMsgLen))))
	//balancer
	balancer.Register(&balancerBuilder{c: clientinstance})
	opts = append(opts, grpc.WithDisableServiceConfig())
	opts = append(opts, grpc.WithDefaultServiceConfig("{\"loadBalancingConfig\":[{\"corelib\":{}}]}"))
	//resolver
	opts = append(opts, grpc.WithResolvers(&resolverBuilder{c: clientinstance}))
	conn, e := grpc.Dial("corelib:///"+serverappname, opts...)
	if e != nil {
		return nil, e
	}
	clientinstance.conn = conn
	return clientinstance, nil
}

type RegisterData struct {
	DServers map[string]struct{} //server register on which discovery server
	Addition []byte
}

//all: key server's addr "ip:port"
func (c *CGrpcClient) UpdateDiscovery(all map[string]*RegisterData) {
	s := resolver.State{
		Addresses: make([]resolver.Address, 0, len(all)),
	}
	for addr, info := range all {
		if len(info.DServers) == 0 {
			continue
		}
		attr := &attributes.Attributes{}
		attr = attr.WithValue("addition", info.Addition)
		attr = attr.WithValue("dservers", info.DServers)
		s.Addresses = append(s.Addresses, resolver.Address{
			Addr:               addr,
			BalancerAttributes: attr,
		})
	}
	c.resolver.cc.UpdateState(s)
}
func (c *CGrpcClient) Call(ctx context.Context, path string, req interface{}, resp interface{}, metadata map[string]string) error {
	start := time.Now()
	if c.c.GlobalTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, start.Add(c.c.GlobalTimeout))
		defer cancel()
	}
	dl, ok := ctx.Deadline()
	if ok && dl.UnixNano() <= start.UnixNano()+int64(5*time.Millisecond) {
		return cerror.ErrDeadlineExceeded
	}
	md := gmetadata.New(nil)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		md.Set("core_metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := trace.GetTrace(ctx)
	if traceid != "" {
		md.Set("core_tracedata", traceid, c.selfappname, selfmethod, selfpath, strconv.Itoa(selfdeep))
	}
	md.Set("core_target", c.serverappname)
	ctx = gmetadata.NewOutgoingContext(ctx, md)
	for {
		p := &peer.Peer{}
		e := transGrpcError(c.conn.Invoke(ctx, path, req, resp, grpc.Peer(p)))
		end := time.Now()
		if p.Addr == nil {
			//pick error or create stream unretryable error,req doesn't send
		} else {
			//req send,recv error
			trace.Trace(ctx, trace.CLIENT, c.serverappname, p.Addr.String(), "GRPC", path, &start, &end, e)
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
	case codes.ResourceExhausted:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unimplemented:
		return cerror.ErrNoapi
	case codes.Internal:
		return cerror.MakeError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unavailable:
		return cerror.MakeError(-1, http.StatusServiceUnavailable, s.Message())
	case codes.Unauthenticated:
		return cerror.MakeError(-1, http.StatusServiceUnavailable, s.Message())
	default:
		ee := cerror.ConvertErrorstr(s.Message())
		ee.SetHttpcode(int32(s.Code()))
		return ee
	}
}