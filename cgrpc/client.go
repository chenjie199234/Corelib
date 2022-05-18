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
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/name"
	"google.golang.org/grpc"
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

//key server's addr "ip:port"
//if the value is nil means this server node is offline
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
	HeartPorbe       time.Duration //default 1s
	MaxMsgLen        uint32        //default 64M,min 64k
	UseTLS           bool          //grpc or grpcs
	SkipVerifyTLS    bool          //don't verify the server's cert
	CAs              []string      //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
	Picker           PickHandler
	Discover         DiscoveryHandler //this function is used to resolve server addrs
	DiscoverInterval time.Duration    //the frequency call Discover to resolve the server addrs,default 10s,min 1s
}

func (c *ClientConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = time.Millisecond * 500
	}
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
	if c.HeartPorbe < time.Second {
		c.HeartPorbe = time.Second
	}
	if c.DiscoverInterval <= 0 {
		c.DiscoverInterval = time.Second * 10
	} else if c.DiscoverInterval < time.Second {
		c.DiscoverInterval = time.Second
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65535 {
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
	opts = append(opts, grpc.WithConnectParams(grpc.ConnectParams{
		MinConnectTimeout: c.ConnectTimeout,
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
	opts = append(opts, grpc.WithResolvers(&resolverBuilder{group: servergroup, name: servername, c: clientinstance}))
	conn, e := grpc.Dial("corelib:///"+serverappname, opts...)
	if e != nil {
		return nil, e
	}
	clientinstance.conn = conn
	return clientinstance, nil
}

func (c *CGrpcClient) ResolveNow() {
	c.resolver.ResolveNow(resolver.ResolveNowOptions{})
}

func (c *CGrpcClient) Call(ctx context.Context, path string, req interface{}, resp interface{}, metadata map[string]string) error {
	if c.c.GlobalTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(c.c.GlobalTimeout))
		defer cancel()
	}
	md := gmetadata.New(nil)
	if len(metadata) != 0 {
		d, _ := json.Marshal(metadata)
		md.Set("core_metadata", common.Byte2str(d))
	}
	traceid, _, _, selfmethod, selfpath, selfdeep := log.GetTrace(ctx)
	if traceid != "" {
		md.Set("core_tracedata", traceid, c.selfappname, selfmethod, selfpath, strconv.Itoa(selfdeep))
	}
	md.Set("core_target", c.serverappname)
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
			log.Trace(ctx, log.CLIENT, c.serverappname, p.Addr.String(), "GRPC", path, &start, &end, e)
			monitor.GrpcClientMonitor(c.serverappname, "GRPC", path, e, uint64(end.UnixNano()-start.UnixNano()))
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
