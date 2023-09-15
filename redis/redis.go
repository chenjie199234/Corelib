package redis

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"

	gredis "github.com/redis/go-redis/v9"
)

type Config struct {
	RedisName string `json:"redis_name"`
	//only support tcp socket
	//ip:port or host:port
	//if there is only one addr,the simple redis client will be created
	//if there are many addrs,the cluster redis client will be created
	Addrs []string `json:"addrs"`
	//username and password is for redis 6.0+'s acl
	//if redis version is under 6.0,only need password
	UserName string `json:"user_name"`
	Password string `json:"password"`
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

// if tlsc is not nil,the tls will be actived
func NewRedis(c *Config, tlsc *tls.Config) (*Client, error) {
	gredisc := &gredis.UniversalOptions{
		ClientName:            c.RedisName,
		Addrs:                 c.Addrs,
		Username:              c.UserName,
		Password:              c.Password,
		DialTimeout:           c.DialTimeout.StdDuration(),
		ReadTimeout:           c.IOTimeout.StdDuration(),
		WriteTimeout:          c.IOTimeout.StdDuration(),
		ContextTimeoutEnabled: true,
		PoolSize:              int(c.MaxOpen),
		MinIdleConns:          1,
		ConnMaxIdleTime:       c.MaxConnIdletime.StdDuration(),
		TLSConfig:             tlsc,
	}
	if c.MaxOpen == 0 {
		gredisc.PoolSize = 100
	}
	if c.DialTimeout <= 0 {
		gredisc.DialTimeout = time.Second * 5
	}
	if c.MaxConnIdletime <= 0 {
		gredisc.ConnMaxIdleTime = -1
	}
	if c.IOTimeout <= 0 {
		gredisc.ReadTimeout = -1
		gredisc.WriteTimeout = -1
	}
	client := &Client{gredis.NewUniversalClient(gredisc)}
	if _, e := client.Ping(context.Background()).Result(); e != nil {
		return nil, e
	}
	//TODO add otel
	return client, nil
}
