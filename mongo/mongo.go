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
	//start the mongodb+srv mode,this will use dns to search mongodb servers' addrs
	//step 1: dns's TXT records: nslookip -qt=txt Addrs[0] => get the connect params,this may contain srvservicename,if not,the srvservicename will be 'mongodb' by default
	//step 2: dns's SRV records: nslookup -qt=srv _srvservicename._tcp.Addrs[0] => get the mongodb servers' addrs and port
	MongoDBSRV bool `json:"mongodb_srv"`
	//if MongoDBSRV is true,Addrs can only contain 1 element and it must be a host without port and scheme
	//if MongoDBSRV is false,this is the mongodb servers' addrs,ip:port or host:port
	Addrs []string `json:"addrs"`
	//only the ReplicaSet cluster need to set this
	//in mongodb+srv mode,this will be overwrite by the dns's TXT records,if the dns's TXT records contain ReplicaSet setting
	ReplicaSet string `json:"replica_set"`
	//default admin
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

// if MongoDBSRV is true or tlsc is not nil,the tls will be actived
// the json tag will be supported
func NewMongo(c *Config, tlsc *tls.Config) (*Client, error) {
	var opts *goptions.ClientOptions
	opts = goptions.Client()
	if c.MongoName != "" {
		opts = opts.SetAppName(c.MongoName)
	}
	opts = opts.SetBSONOptions(&goptions.BSONOptions{UseJSONStructTags: true})
	if c.ReplicaSet != "" {
		opts = opts.SetReplicaSet(c.ReplicaSet)
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
	if c.MongoDBSRV {
		if len(c.Addrs) != 1 || strings.Contains(c.Addrs[0], ":") || strings.Contains(c.Addrs[0], ",") {
			return nil, errors.New("when mongodb_srv is true,addrs can only contain 1 element and it must be a host without port and scheme")
		}
		//start the mongodb+srv mode
		//this will overwrite the above setting if the dns's TXT records have
		if c.UserName != "" && c.Password != "" {
			if c.AuthDB != "" {
				//specific the authSource
				opts = opts.ApplyURI("mongodb+srv://" + c.UserName + ":" + c.Password + "@" + c.Addrs[0] + "/?authSource=" + c.AuthDB)
			} else {
				//use the authSource in dns's TXT records or use the default 'admin'
				opts = opts.ApplyURI("mongodb+srv://" + c.UserName + ":" + c.Password + "@" + c.Addrs[0])
			}
		} else {
			opts = opts.ApplyURI("mongodb+srv://" + c.Addrs[0])
		}
	} else {
		if len(c.Addrs) == 0 {
			return nil, errors.New("missing addrs")
		}
		opts = opts.SetHosts(c.Addrs)
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
	}
	//if we have the specific tls config,then use the specific one
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
