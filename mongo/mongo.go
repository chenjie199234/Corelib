package mongo

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"

	gmongo "go.mongodb.org/mongo-driver/mongo"
	goptions "go.mongodb.org/mongo-driver/mongo/options"
	greadpref "go.mongodb.org/mongo-driver/mongo/readpref"
)

type Config struct {
	MongoName string `json:"mongo_name"`
	//if this is not empty,mongodb+srv mode will be actived
	//this is used to search mongodb servers' addrs from dns's SRV records,e.g.: nslookup -qt=srv _SRVName._tcp.Addr[0]
	//this is also used to search mongodb servers' connect prarms from dns's TXT records,e.g.: nslookup -qt=txt _SRVName._tcp.Addr[0]
	//if you want to use mongodb+srv mode and you don't known the SRVName,try to set it to 'mongodb'
	SRVName string `json:"srv_name"`
	//if SRVName is not empty,Addrs can only contain 1 element and it must be a host without port and scheme
	//if SRVName is empty,this is the mongodb servers' addrs,ip:port or host:port
	Addrs []string `json:"addrs"`
	//only the ReplicaSet cluster need to set this
	//Warning!In mongodb+srv mode,this will overwrite the connect params from dns's TXT records,see comment in SRVName
	ReplicaSet string `json:"replica_set"`
	//default admin
	//Warning!In mongodb+srv mode,this will overwrite the connect params from dns's TXT records,see comment in SRVName
	AuthDB   string `json:"auth_db"`
	UserName string `json:"user_name"`
	Password string `json:"password"`
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
	opts = opts.SetBSONOptions(&goptions.BSONOptions{UseJSONStructTags: true})
	if c.SRVName != "" {
		opts = opts.SetSRVServiceName(c.SRVName)
		if len(c.Addrs) != 1 {
			return nil, errors.New("addrs can only has 1 element when srv_name is not empty")
		}
		if strings.Contains(c.Addrs[0], ",") || strings.Contains(c.Addrs[0], ":") {
			return nil, errors.New("addr must be a host without port and scheme when srv_name is not empty")
		}
		opts = opts.ApplyURI("mongodb+srv://" + c.Addrs[0])
	} else {
		opts = opts.SetHosts(c.Addrs)
	}
	opts = opts.SetReplicaSet(c.ReplicaSet)
	if c.UserName != "" && c.Password != "" {
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
