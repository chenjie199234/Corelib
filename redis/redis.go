package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/trace"
	"github.com/chenjie199234/Corelib/util/ctime"

	gredis "github.com/redis/go-redis/v9"
)

type Config struct {
	RedisName string `json:"redis_name"`
	//direct: Addrs can only contain 1 addr
	//sentinel: see redis's sentinel mode(https://redis.io/docs/management/sentinel/)
	//	Warning!Sentinel node and Redis node must use the same auth setting and tls setting
	//cluster: see redis's cluster mode(https://redis.io/docs/reference/cluster-spec/)
	//ring: see go-redis's ring mode(https://github.com/redis/go-redis)
	//randomring: this is the ring mode without consistent hash,redis cmds will route randomly
	//	when connect to multi proxys for one cluster,this mode is useful
	RedisMode string `json:"redis_mode"`
	//only support tcp socket,ip:port or host:port
	Addrs []string `json:"addrs"`
	//UserName and Password is for redis's acl(start from 6.0) and sentinel's acl(start from 6.2)
	//if redis version is under 6.0 or sentinel's version under 6.2,only need Password
	UserName string `json:"user_name"`
	Password string `json:"password"`
	//only useful when RedisMode is sentinel,and this is required
	SentinelMasterName string `json:"sentinel_master_name"`
	//only useful when RedisMode is sentinel or cluster
	ReadWriteSplit bool `json:"read_write_split"`
	//0: default 100
	MaxOpen uint16 `json:"max_open"`
	//<=0: connection has no idle timeout
	MaxConnIdletime ctime.Duration `json:"max_conn_idletime"`
	//<=0: default 5s
	DialTimeout ctime.Duration `json:"dial_timeout"`
	//<=0: no timeout
	IOTimeout ctime.Duration `json:"io_timeout"`
}

type Client struct {
	gredis.UniversalClient
}

type _proxys []string

func (p _proxys) Get(string) string {
	return p[rand.Intn(len(p))]
}

// if tlsc is not nil,the tls will be actived
func NewRedis(c *Config, tlsc *tls.Config) (*Client, error) {
	if len(c.Addrs) == 0 {
		c.Addrs = []string{"127.0.0.1:6379"}
	}
	if len(c.Addrs) > 1 {
		undup := make(map[string]*struct{}, len(c.Addrs))
		for _, addr := range c.Addrs {
			undup[addr] = nil
		}
		tmp := make([]string, 0, len(undup))
		for addr := range undup {
			tmp = append(tmp, addr)
		}
		c.Addrs = tmp
	}
	if c.MaxOpen == 0 {
		c.MaxOpen = 100
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = ctime.Duration(time.Second * 5)
	}
	if c.MaxConnIdletime <= 0 {
		//disable close idle conns
		c.MaxConnIdletime = -1
	}
	if c.IOTimeout <= 0 {
		//disable read and write timeout
		c.IOTimeout = -1
	}
	var client *Client
	switch c.RedisMode {
	case "direct":
		if len(c.Addrs) != 1 {
			return nil, errors.New("too many addrs in config for direct mode")
		}
		client = &Client{gredis.NewClient(&gredis.Options{
			ClientName:            c.RedisName,
			Addr:                  c.Addrs[0],
			Username:              c.UserName,
			Password:              c.Password,
			DialTimeout:           c.DialTimeout.StdDuration(),
			ReadTimeout:           c.IOTimeout.StdDuration(),
			WriteTimeout:          c.IOTimeout.StdDuration(),
			ContextTimeoutEnabled: true,
			PoolSize:              int(c.MaxOpen),
			MaxActiveConns:        int(c.MaxOpen),
			MinIdleConns:          1,
			ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
			TLSConfig:             tlsc,
		})}
	case "sentinel":
		if c.SentinelMasterName == "" {
			return nil, errors.New("missing sentinel_master_name in config for sentinel mode")
		}
		client = &Client{gredis.NewFailoverClusterClient(&gredis.FailoverOptions{
			ClientName:            c.RedisName,
			MasterName:            c.SentinelMasterName,
			SentinelAddrs:         c.Addrs,
			SentinelUsername:      c.UserName,
			SentinelPassword:      c.Password,
			RouteRandomly:         c.ReadWriteSplit,
			Username:              c.UserName,
			Password:              c.Password,
			DialTimeout:           c.DialTimeout.StdDuration(),
			ReadTimeout:           c.IOTimeout.StdDuration(),
			WriteTimeout:          c.IOTimeout.StdDuration(),
			ContextTimeoutEnabled: true,
			PoolSize:              int(c.MaxOpen),
			MaxActiveConns:        int(c.MaxOpen),
			MinIdleConns:          1,
			ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
			TLSConfig:             tlsc,
		})}
	case "cluster":
		if len(c.Addrs) == 1 {
			return nil, errors.New("too less addrs in config for cluster mode")
		}
		client = &Client{gredis.NewClusterClient(&gredis.ClusterOptions{
			ClientName:            c.RedisName,
			Addrs:                 c.Addrs,
			RouteRandomly:         c.ReadWriteSplit,
			Username:              c.UserName,
			Password:              c.Password,
			DialTimeout:           c.DialTimeout.StdDuration(),
			ReadTimeout:           c.IOTimeout.StdDuration(),
			WriteTimeout:          c.IOTimeout.StdDuration(),
			ContextTimeoutEnabled: true,
			PoolSize:              int(c.MaxOpen),
			MaxActiveConns:        int(c.MaxOpen),
			MinIdleConns:          1,
			ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
			TLSConfig:             tlsc,
		})}
	case "ring":
		if len(c.Addrs) == 1 {
			return nil, errors.New("too less addrs in config for ring mode")
		}
		addrs := make(map[string]string, len(c.Addrs))
		for i, addr := range c.Addrs {
			addrs["node"+strconv.Itoa(i+1)] = addr
		}
		client = &Client{gredis.NewRing(&gredis.RingOptions{
			ClientName:            c.RedisName,
			Addrs:                 addrs,
			Username:              c.UserName,
			Password:              c.Password,
			DialTimeout:           c.DialTimeout.StdDuration(),
			ReadTimeout:           c.IOTimeout.StdDuration(),
			WriteTimeout:          c.IOTimeout.StdDuration(),
			ContextTimeoutEnabled: true,
			PoolSize:              int(c.MaxOpen),
			MaxActiveConns:        int(c.MaxOpen),
			MinIdleConns:          1,
			ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
			TLSConfig:             tlsc,
		})}
	case "randomring":
		if len(c.Addrs) == 1 {
			return nil, errors.New("too less addrs in config for randomring mode")
		}
		addrs := make(map[string]string, len(c.Addrs))
		for i, addr := range c.Addrs {
			addrs["node"+strconv.Itoa(i+1)] = addr
		}
		client = &Client{gredis.NewRing(&gredis.RingOptions{
			ClientName:            c.RedisName,
			Addrs:                 addrs,
			NewConsistentHash:     func(proxys []string) gredis.ConsistentHash { return _proxys(proxys) },
			Username:              c.UserName,
			Password:              c.Password,
			DialTimeout:           c.DialTimeout.StdDuration(),
			ReadTimeout:           c.IOTimeout.StdDuration(),
			WriteTimeout:          c.IOTimeout.StdDuration(),
			ContextTimeoutEnabled: true,
			PoolSize:              int(c.MaxOpen),
			MaxActiveConns:        int(c.MaxOpen),
			MinIdleConns:          1,
			ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
			TLSConfig:             tlsc,
		})}
	default:
		return nil, errors.New("unknown redis mode")
	}
	switch rdb := client.UniversalClient.(type) {
	case *gredis.Client:
		opts := rdb.Options()
		rdb.AddHook(newMonitor(c.RedisName, opts.Addr))
	case *gredis.ClusterClient:
		rdb.OnNewNode(func(srdb *gredis.Client) {
			srdb.AddHook(newMonitor(c.RedisName, srdb.Options().Addr))
		})
	case *gredis.Ring:
		rdb.OnNewNode(func(srdb *gredis.Client) {
			srdb.AddHook(newMonitor(c.RedisName, srdb.Options().Addr))
		})
	}
	if _, e := client.Ping(context.Background()).Result(); e != nil {
		return nil, e
	}
	return client, nil
}

// only when the redis mode is direct
func (c *Client) GetClient() *gredis.Client {
	cc, ok := c.UniversalClient.(*gredis.Client)
	if ok {
		return cc
	}
	return nil
}

// only when the redis mode is sentinel or cluter
func (c *Client) GetClusterClient() *gredis.ClusterClient {
	cc, ok := c.UniversalClient.(*gredis.ClusterClient)
	if ok {
		return cc
	}
	return nil
}

// only when the redis mode is ring or randomring
func (c *Client) GetRingClient() *gredis.Ring {
	cc, ok := c.UniversalClient.(*gredis.Ring)
	if ok {
		return cc
	}
	return nil
}

// ----------------------------Monitor-----------------------------------
type monitor struct {
	redisname string
	addr      string
}

func newMonitor(redisname string, addr string) *monitor {
	return &monitor{
		redisname: redisname,
		addr:      addr,
	}
}
func (m *monitor) DialHook(hook gredis.DialHook) gredis.DialHook {
	return hook
}
func (m *monitor) ProcessHook(hook gredis.ProcessHook) gredis.ProcessHook {
	return func(ctx context.Context, cmd gredis.Cmder) error {
		ctx, span := trace.NewSpan(ctx, "Corelib.Redis", trace.Client, nil)
		span.GetSelfSpanData().SetStateKV("redis", m.redisname)
		span.GetSelfSpanData().SetStateKV("host", m.addr)
		span.GetSelfSpanData().SetStateKV("cmd", cmd.FullName())
		e := hook(ctx, cmd)
		span.Finish(e)
		return e
	}
}
func (m *monitor) ProcessPipelineHook(hook gredis.ProcessPipelineHook) gredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []gredis.Cmder) error {
		ctx, span := trace.NewSpan(ctx, "Corelib.Redis", trace.Client, nil)
		span.GetSelfSpanData().SetStateKV("redis", m.redisname)
		span.GetSelfSpanData().SetStateKV("host", m.addr)
		cmdnames := make([]string, 0, len(cmds))
		for _, cmd := range cmds {
			cmdnames = append(cmdnames, cmd.FullName())
		}
		span.GetSelfSpanData().SetStateKV("cmds", strings.Join(cmdnames, ","))
		e := hook(ctx, cmds)
		span.Finish(e)
		return e
	}
}
