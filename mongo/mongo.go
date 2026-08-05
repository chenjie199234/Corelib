package mongo

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/ctime"

	gbson "go.mongodb.org/mongo-driver/v2/bson"
	gevent "go.mongodb.org/mongo-driver/v2/event"
	gmongo "go.mongodb.org/mongo-driver/v2/mongo"
	goptions "go.mongodb.org/mongo-driver/v2/mongo/options"
	greadpref "go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	//<=0: no timeout,context's deadline > IOTimeout > context without deadline
	IOTimeout ctime.Duration `json:"io_timeout"`
}

func (c *Config) clone() *Config {
	return &Config{
		MongoName:       c.MongoName,
		MongoDBSRV:      c.MongoDBSRV,
		Addrs:           c.Addrs,
		ReplicaSet:      c.ReplicaSet,
		AuthDB:          c.AuthDB,
		UserName:        c.UserName,
		Password:        c.Password,
		MaxOpen:         c.MaxOpen,
		MaxConnIdletime: c.MaxConnIdletime,
		DialTimeout:     c.DialTimeout,
		IOTimeout:       c.IOTimeout,
	}
}

type Client struct {
	*gmongo.Client
}

// if MongoDBSRV is true or tlsc is not nil,the tls will be activated
// the json tag will be supported
func NewMongo(c *Config, tlsc *tls.Config) (*Client, error) {
	c = c.clone()
	if len(c.Addrs) == 0 {
		c.Addrs = []string{"127.0.0.1:27017"}
	}
	if len(c.Addrs) > 1 {
		undup := make(map[string]struct{}, len(c.Addrs))
		for _, addr := range c.Addrs {
			undup[addr] = struct{}{}
		}
		tmp := make([]string, 0, len(undup))
		for addr := range undup {
			tmp = append(tmp, addr)
		}
		c.Addrs = tmp
	}
	var opts *goptions.ClientOptions
	opts = goptions.Client()
	if c.MongoName != "" {
		opts = opts.SetAppName(c.MongoName)
	}
	opts = opts.SetMonitor(newMonitor(c.MongoName))
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
	if c.AuthDB == "" {
		c.AuthDB = "admin"
	}
	if (c.UserName != "") != (c.Password != "") {
		return nil, errors.New("username and password must both be empty or not")
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
		opts = opts.ApplyURI("mongodb+srv://" + c.Addrs[0])
		if c.UserName != "" && c.Password != "" {
			opts = opts.SetAuth(goptions.Credential{
				AuthSource: c.AuthDB,
				Username:   c.UserName,
				Password:   c.Password,
			})
		}
	} else {
		opts = opts.SetHosts(c.Addrs)
		if c.UserName != "" && c.Password != "" {
			opts = opts.SetAuth(goptions.Credential{
				AuthSource: c.AuthDB,
				Username:   c.UserName,
				Password:   c.Password,
			})
		}
	}
	//if we have the specific tls config,then use the specific one
	if tlsc != nil {
		opts = opts.SetTLSConfig(tlsc)
	}
	client, e := gmongo.Connect(opts)
	if e != nil {
		return nil, e
	}
	if e = client.Ping(context.Background(), greadpref.Primary()); e != nil {
		client.Disconnect(context.Background())
		return nil, e
	}
	return &Client{client}, nil
}

// ----------------------------Monitor-----------------------------------
type monitor struct {
	sync.Mutex
	mongoname string
	spans     map[string]trace.Span
	tracer    trace.Tracer
}

func newMonitor(mongoname string) *gevent.CommandMonitor {
	m := &monitor{
		mongoname: mongoname,
		spans:     make(map[string]trace.Span),
		tracer:    otel.Tracer("Corelib.mongo.client", trace.WithInstrumentationVersion(version.String())),
	}
	return &gevent.CommandMonitor{
		Started:   m.Started,
		Succeeded: m.Succeeded,
		Failed:    m.Failed,
	}
}
func (m *monitor) Started(ctx context.Context, evt *gevent.CommandStartedEvent) {
	if evt.CommandName == "getMore" {
		return
	}
	hostname, port := peerInfo(evt)
	_, span := m.tracer.Start(ctx, "call mongodb", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", m.mongoname),
			attribute.String("sip", hostname+":"+strconv.Itoa(port)),
			attribute.String("mongo.db", evt.DatabaseName),
			attribute.String("mongo.col", colInfo(evt)),
			attribute.String("mongo.cmd", evt.CommandName),
		),
	)
	m.Lock()
	defer m.Unlock()
	m.spans[evt.ConnectionID+"-"+strconv.FormatInt(evt.RequestID, 10)] = span
}
func (m *monitor) Succeeded(ctx context.Context, evt *gevent.CommandSucceededEvent) {
	m.Finished(&evt.CommandFinishedEvent, nil)
}
func (m *monitor) Failed(ctx context.Context, evt *gevent.CommandFailedEvent) {
	m.Finished(&evt.CommandFinishedEvent, evt.Failure)
}
func (m *monitor) Finished(evt *gevent.CommandFinishedEvent, err error) {
	m.Lock()
	key := evt.ConnectionID + "-" + strconv.FormatInt(evt.RequestID, 10)
	span, ok := m.spans[key]
	if ok {
		delete(m.spans, key)
	}
	m.Unlock()
	if !ok {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

func colInfo(evt *gevent.CommandStartedEvent) string {
	elt, err := evt.Command.IndexErr(0)
	if err != nil {
		return "collection unknown"
	}
	if key, err := elt.KeyErr(); err == nil && key == evt.CommandName {
		var v gbson.RawValue
		if v, err = elt.ValueErr(); err != nil || v.Type != gbson.TypeString {
			return "collection unknown"
		}
		return v.StringValue()
	}
	return "collection unknown"
}

// peerInfo will parse the hostname and port from the mongo connection ID.
//
// ConnectionID is always formatted by the driver as "<address>[-<connection
// number>]", so the pooled connection number suffix can be stripped by
// cutting at the first "[-".
// See https://github.com/open-telemetry/opentelemetry-go-contrib/issues/9370.
func peerInfo(evt *gevent.CommandStartedEvent) (hostname string, port int) {
	defaultMongoPort := 27017
	host, _, _ := strings.Cut(evt.ConnectionID, "[-")
	hostname, portStr, err := net.SplitHostPort(host)
	if err != nil {
		// If parsing fails, assume default MongoDB port and return the entire host as hostname
		hostname = host
		port = defaultMongoPort
		return hostname, port
	}

	port, err = strconv.Atoi(portStr)
	if err != nil || port < 1 {
		// If port parsing fails, fallback to default MongoDB port
		port = defaultMongoPort
	}

	return hostname, port
}
