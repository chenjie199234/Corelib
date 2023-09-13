package redis

import (
	"crypto/tls"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	RedisName string `json:"redis_name"`
	//ip:port or host:port
	//if there is only one addr,the simple redis client will be created
	//if there are many addrs,the cluster redis client will be created
	Addrs []string `json:"addrs"`
	//username and password is for redis 6.0+'s acl
	//if redis version is under 6.0,only need password
	UserName string `json:"user_name"`
	Password string `json:"password"`
	//this is the pool's buf for every addr
	//if this is 0,no connections will be reused
	//because the pool's buf is 0,no connections can be buffed
	MaxIdle     uint16        `json:"max_idle"`
	MaxOpen     uint16        `json:"max_open"`
	MaxIdletime time.Duration `json:"max_idletime"`
	ConnTimeout time.Duration `json:"conn_timeout"`
	IOTimeout   time.Duration `json:"io_time"`
}

type Client struct {
	redis.UniversalClient
}

// if tlsc is not nil,the tls will be actived
func NewRedis(c *Config, tlsc *tls.Config) *Client {
	client := &Client{redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:                 c.Addrs,
		ClientName:            c.RedisName,
		Username:              c.UserName,
		Password:              c.Password,
		DialTimeout:           c.ConnTimeout,
		ReadTimeout:           c.IOTimeout,
		WriteTimeout:          c.IOTimeout,
		ContextTimeoutEnabled: true,
		PoolSize:              int(c.MaxOpen),
		MaxIdleConns:          int(c.MaxIdle),
		ConnMaxIdleTime:       c.MaxIdletime,
		TLSConfig:             tlsc,
	})}
	//TODO add otel
	return client
}
