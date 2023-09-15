package mongo

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"

	gmongo "go.mongodb.org/mongo-driver/mongo"
	goptions "go.mongodb.org/mongo-driver/mongo/options"
	greadpref "go.mongodb.org/mongo-driver/mongo/readpref"
)

type Config struct {
	MongoName string `json:"mongo_name"`
	//only support tcp socket
	//ip:port or host:port
	Addrs []string `json:"addrs"`
	//only the replica set mode need to set this
	//shard mode and standalone mode set this empty
	ReplicaSet string `json:"replica_set"`
	UserName   string `json:"user_name"`
	Password   string `json:"password"`
	//default admin
	AuthDB string `json:"auth_db"`
	//0: default 100
	MaxOpen uint16 `json:"max_open"`
	//<=0: no idletime
	MaxConnIdletime ctime.Duration `json:"max_conn_idletime"`
	//<=0: default 5s
	DialTimeout ctime.Duration `json:"dial_timeout"`
	//<=0: no timeout
	IOTimeout ctime.Duration `json:"io_timeout"`
}
type Client struct {
	*gmongo.Client
}

// if tlsc is not nil,the tls will be actived
func NewMongo(c *Config, tlsc *tls.Config) (*Client, error) {
	var opts *goptions.ClientOptions
	opts = goptions.Client()
	if c.MongoName != "" {
		opts = opts.SetAppName(c.MongoName)
	}
	opts = opts.SetHosts(c.Addrs)
	opts = opts.SetReplicaSet(c.ReplicaSet)
	if c.UserName != "" && c.Password != "" {
		if c.AuthDB == "" {
			c.AuthDB = "admin"
		}
		opts = opts.SetAuth(goptions.Credential{
			AuthMechanism:           "",
			AuthMechanismProperties: nil,
			AuthSource:              c.AuthDB,
			Username:                c.UserName,
			Password:                c.Password,
			PasswordSet:             true,
		})
	}
	opts = opts.SetMinPoolSize(1)
	if c.MaxOpen == 0 {
		opts = opts.SetMaxPoolSize(100)
	} else {
		opts = opts.SetMaxPoolSize(uint64(c.MaxOpen))
	}
	if c.MaxConnIdletime > 0 {
		opts = opts.SetMaxConnIdleTime(c.MaxConnIdletime.StdDuration())
	} else {
		opts = opts.SetMaxConnIdleTime(0)
	}
	if c.DialTimeout <= 0 {
		opts = opts.SetConnectTimeout(time.Second * 5)
	} else {
		opts = opts.SetConnectTimeout(c.DialTimeout.StdDuration())
	}
	if c.IOTimeout > 0 {
		opts = opts.SetTimeout(c.IOTimeout.StdDuration())
	}
	if tlsc != nil {
		opts = opts.SetTLSConfig(tlsc)
	}
	client, e := gmongo.Connect(context.Background(), opts)
	if e != nil {
		return nil, e
	}
	if e = client.Ping(context.Background(), greadpref.Primary()); e != nil {
		return nil, e
	}
	//TODO add otel
	return &Client{client}, nil
}
