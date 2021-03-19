package config

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

const text = `package config

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"{{.}}/api"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/redis"
	ctime "github.com/chenjie199234/Corelib/util/time"
	"github.com/fsnotify/fsnotify"
	"github.com/go-sql-driver/mysql"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

//AppConfig can hot update
//this is the config used for this app
type AppConfig struct {
	//add your config here
}
//EnvConfig can't hot update,all these data is from system env setting
//each field:nil means system env not exist
type EnvConfig struct {
	DiscoveryGroup    *string
	DiscoveryName     *string
	DiscoveryPort     *int
	ServerVerifyDatas []string
	RemoteConfig      *bool
	RunEnv            *string
	DeployEnv         *string
}
//sourceConfig can't hot update
type sourceConfig struct {
	Rpc      *RpcConfig                 $json:"rpc"$
	Web      *WebConfig                 $json:"web"$
	Mongo    map[string]*MongoConfig    $json:"mongo"$
	Sql      map[string]*SqlConfig      $json:"sql"$       //key example:xx_sql
	Redis    map[string]*RedisConfig    $json:"redis"$     //key example:xx_redis
	KafkaPub map[string]*KafkaPubConfig $json:"kafka_pub"$ //key:topic name
	KafkaSub map[string]*KafkaSubConfig $json:"kafka_sub"$ //key:topic name
}

//RpcConfig -
type RpcConfig struct {
	RpcTimeout      ctime.Duration $json:"rpc_timeout"$       //default 500ms
	RpcConnTimeout  ctime.Duration $json:"rpc_conn_timeout"$  //default 1s
	RpcHeartTimeout ctime.Duration $json:"rpc_heart_timeout"$ //default 5s
	RpcHeartProbe   ctime.Duration $json:"rpc_heart_probe"$   //default 1.5s
}

//WebConfig -
type WebConfig struct {
	//server
	WebTimeout      ctime.Duration $json:"web_timeout"$ //default 500ms
	WebStaticFile   string         $json:"web_staticfile"$
	WebCertFile     string         $json:"web_certfile"$
	WebKeyFile      string         $json:"web_keyfile"$
	//cors
	WebCors *WebCorsConfig $json:"web_cors"$
}

//WebCorsConfig -
type WebCorsConfig struct {
	CorsOrigin []string $json:"cors_origin"$
	CorsHeader []string $json:"cors_header"$
	CorsExpose []string $json:"cors_expose"$
}

//RedisConfig -
type RedisConfig struct {
	Username    string         $json:"username"$
	Passwd      string         $json:"passwd"$
	Addr        string         $json:"addr"$
	MaxOpen     int            $json:"max_open"$     //default 100
	MaxIdletime ctime.Duration $json:"max_idletime"$ //default 10min
	IoTimeout   ctime.Duration $json:"io_timeout"$   //default 500ms
	ConnTimeout ctime.Duration $json:"conn_timeout"$ //default 250ms
}

//SqlConfig -
type SqlConfig struct {
	Username    string         $json:"username"$
	Passwd      string         $json:"passwd"$
	Net         string         $json:"net"$
	Addr        string         $json:"addr"$
	Collation   string         $json:"collation"$
	MaxOpen     int            $json:"max_open"$     //default 100
	MaxIdletime ctime.Duration $json:"max_idletime"$ //default 10min
	IoTimeout   ctime.Duration $json:"io_timeout"$   //default 500ms
	ConnTimeout ctime.Duration $json:"conn_timeout"$ //default 250ms
}

//MongoConfig -
type MongoConfig struct {
	Username       string         $json:"username"$
	Passwd         string         $json:"passwd"$
	Addrs          []string       $json:"addrs"$
	ReplicaSetName string         $json:"replica_set_name"$
	MaxOpen        uint64         $json:"max_open"$     //default 100
	MaxIdletime    ctime.Duration $json:"max_idletime"$ //default 10min
	IoTimeout      ctime.Duration $json:"io_timeout"$   //default 500ms
	ConnTimeout    ctime.Duration $json:"conn_timeout"$ //default 250ms
}

//KafkaPubConfig -
type KafkaPubConfig struct {
	Addr     string $json:"addr"$
	Username string $json:"username"$
	Passwd   string $json:"password"$
}

//KafkaSubConfig -
type KafkaSubConfig struct {
	Addr      string $json:"addr"$
	Username  string $json:"username"$
	Passwd    string $json:"password"$
	GroupName string $json:"group_name"$
	//-1 will sub from the newest
	//-2 will sub from the firt
	//if this is 0,default -2 will be used
	StartOffset int64 $json:"start_offset"$
	//if this is 0,commit is synced,and effective is slow.
	//if this is not 0,commit is asynced,effective is high,but will cause duplicate sub when the program crash
	CommitInterval ctime.Duration $json:"commit_interval"$
}

//AC -
var AC *AppConfig

var watcher *fsnotify.Watcher
var closech chan struct{}

//EC -
var EC *EnvConfig

//SC total source config instance
var sc *sourceConfig

var mongos map[string]*mongo.Client

var sqls map[string]*sql.DB

var rediss map[string]*redis.Pool

var kafkaSubers map[string]*kafka.Reader

var kafkaPubers map[string]*kafka.Writer

func init() {
	initenv()
	initdiscovery()
	initremote()
	initsource()
	initapp()
}

func initenv(){
	EC = &EnvConfig{}
	if str, ok := os.LookupEnv("SERVER_VERIFY_DATA"); ok && str != "<SERVER_VERIFY_DATA>" {
		temp := make([]string, 0)
		if str != "" {
			EC.ServerVerifyDatas = temp
		} else if e := json.Unmarshal([]byte(str), &temp); e!= nil {
			log.Error("[config.initenv] SERVER_VERIFY_DATA must be json string array like:[\"abc\",\"123\"]")
			Close()
			os.Exit(1)
		}
		EC.ServerVerifyDatas = temp
	} else {
		log.Warning("[config.initenv] missing SERVER_VERIFY_DATA")
	}
	if str, ok := os.LookupEnv("DISCOVERY_SERVER_GROUP"); ok && str != "<DISCOVERY_SERVER_GROUP>" && str != "" {
		EC.DiscoveryGroup = &str
	} else if EC.ServerVerifyDatas != nil {
		log.Error("[config.initenv] missing DISCOVERY_SERVER_GROUP")
		Close()
		os.Exit(1)
	} else {
		log.Warning("[config.initenv] missing DISCOVERY_SERVER_GROUP")
	}
	if str, ok := os.LookupEnv("DISCOVERY_SERVER_NAME"); ok && str != "<DISCOVERY_SERVER_NAME>" && str != "" {
		EC.DiscoveryName = &str
	} else if EC.ServerVerifyDatas != nil {
		log.Error("[config.initenv] missing DISCOVERY_SERVER_NAME")
		Close()
		os.Exit(1)
	} else {
		log.Warning("[config.initenv] missing DISCOVERY_SERVER_NAME")
	}
	if str, ok := os.LookupEnv("DISCOVERY_SERVER_PORT"); ok && str != "<DISCOVERY_SERVER_PORT>" && str != "" {
		port, e := strconv.Atoi(str)
		if e != nil {
			log.Error("[config.initenv] DISCOVERY_SERVER_PORT must be number in [1-65535]")
			Close()
			os.Exit(1)
		}
		EC.DiscoveryPort = &port
	} else if EC.ServerVerifyDatas != nil {
		log.Error("[config.initenv] missing DISCOVERY_SERVER_PORT")
		Close()
		os.Exit(1)
	} else {
		log.Warning("[config.initenv] missing DISCOVERY_SERVER_PORT")
	}
	if str, ok := os.LookupEnv("REMOTE_CONFIG"); ok && str != "<REMOTE_CONFIG>" && str != "" {
		use := false
		if strings.ToLower(str) == "true" {
			use = true
		}
		EC.RemoteConfig = &use
	} else {
		log.Warning("[config.initenv] missing REMOTE_CONFIG")
	}
	if str, ok := os.LookupEnv("RUN_ENV"); ok && str != "<RUN_ENV>" && str != "" {
		EC.RunEnv = &str
	} else {
		log.Warning("[config.initenv] missing RUN_ENV")
	}
	if str, ok := os.LookupEnv("DEPLOY_ENV"); ok && str != "<DEPLOY_ENV>" && str != "" {
		EC.DeployEnv = &str
	} else {
		log.Warning("[config.initenv] missing DEPLOY_ENV")
	}
}
func initdiscovery() {
	if EC.ServerVerifyDatas != nil {
		vd := ""
		if len(EC.ServerVerifyDatas) > 0 {
			vd = EC.ServerVerifyDatas[0]
		}
		if e := discovery.NewDiscoveryClient(nil, api.Group, api.Name, vd, discovery.MakeDefaultFinder(*EC.DiscoveryGroup, *EC.DiscoveryName, *EC.DiscoveryPort)); e != nil {
			log.Error("[config.initdiscovery] error:", e)
			Close()
			os.Exit(1)
		}
	}
}
func initremote() {
	if EC.RemoteConfig != nil && *EC.RemoteConfig == true {
		//get remote config
	}
}
func initsource() {
	data, e := os.ReadFile("SourceConfig.json")
	if e != nil {
		log.Error("[config.initsource] read config file error:", e)
		Close()
		os.Exit(1)
	}
	sc = &sourceConfig{}
	if e = json.Unmarshal(data, sc); e != nil {
		log.Error("[config.initsource] config file format error:", e)
		Close()
		os.Exit(1)
	}
	if sc.Rpc.RpcTimeout == 0 {
		sc.Rpc.RpcTimeout = ctime.Duration(time.Millisecond * 500)
	}
	if sc.Rpc.RpcConnTimeout == 0 {
		sc.Rpc.RpcConnTimeout = ctime.Duration(time.Second)
	}
	if sc.Rpc.RpcHeartTimeout == 0 {
		sc.Rpc.RpcHeartTimeout = ctime.Duration(5 * time.Second)
	}
	if sc.Rpc.RpcHeartProbe == 0 {
		sc.Rpc.RpcHeartProbe = ctime.Duration(1500 * time.Millisecond)
	}
	if sc.Web.WebTimeout == 0 {
		sc.Web.WebTimeout = ctime.Duration(time.Millisecond * 500)
	}
	for _, mongoc := range sc.Mongo {
		if mongoc.Username == "" {
			mongoc.Username = ""
			mongoc.Passwd = ""
		}
		if len(mongoc.Addrs) == 0 {
			mongoc.Addrs = []string{"127.0.0.1:27017"}
		}
		if mongoc.MaxOpen == 0 {
			mongoc.MaxOpen = 100
		}
		if mongoc.MaxIdletime == 0 {
			mongoc.MaxIdletime = ctime.Duration(time.Minute * 10)
		}
		if mongoc.IoTimeout == 0 {
			mongoc.IoTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if mongoc.ConnTimeout == 0 {
			mongoc.ConnTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	for _, sqlc := range sc.Sql {
		if sqlc.Username == "" {
			sqlc.Username = "root"
			sqlc.Passwd = ""
		}
		if sqlc.Addr == "" || sqlc.Net == "" {
			sqlc.Addr = "127.0.0.1:3306"
			sqlc.Net = "tcp"
		}
		if sqlc.MaxOpen == 0 {
			sqlc.MaxOpen = 100
		}
		if sqlc.MaxIdletime == 0 {
			sqlc.MaxIdletime = ctime.Duration(time.Minute * 10)
		}
		if sqlc.IoTimeout == 0 {
			sqlc.IoTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sqlc.ConnTimeout == 0 {
			sqlc.ConnTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	for _, redisc := range sc.Redis {
		if redisc.Addr == "" {
			redisc.Addr = "127.0.0.1:6379"
		}
		if redisc.MaxOpen == 0 {
			redisc.MaxOpen = 100
		}
		if redisc.MaxIdletime == 0 {
			redisc.MaxIdletime = ctime.Duration(time.Minute * 10)
		}
		if redisc.IoTimeout == 0 {
			redisc.IoTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if redisc.ConnTimeout == 0 {
			redisc.ConnTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	for _, pubc := range sc.KafkaPub {
		if pubc.Addr == "" {
			pubc.Addr = "127.0.0.1:9092"
		}
		if pubc.Username == "" && pubc.Passwd == "" {
			pubc.Username = "root"
			pubc.Passwd = "root"
		}
	}
	for topic, subc := range sc.KafkaSub {
		if subc.Addr == "" {
			subc.Addr = "127.0.0.1:9092"
		}
		if subc.Username == "" && subc.Passwd == "" {
			subc.Username = "root"
			subc.Passwd = "root"
		}
		if subc.GroupName == "" {
			log.Error("[config.initsource] sub topic:", topic, "groupname missing")
			Close()
			os.Exit(1)
		}
	}
	mongos = make(map[string]*mongo.Client, len(sc.Mongo))
	for k, mongoc := range sc.Mongo {
		if k == "example_mongo" {
			continue
		}
		op := &options.ClientOptions{}
		if mongoc.Username != "" && mongoc.Passwd != "" {
			op = op.SetAuth(options.Credential{Username: mongoc.Username, Password: mongoc.Passwd})
		}
		if mongoc.ReplicaSetName != "" {
			op.SetReplicaSet(mongoc.ReplicaSetName)
		}
		op = op.SetHosts(mongoc.Addrs)
		op = op.SetConnectTimeout(time.Duration(mongoc.ConnTimeout))
		op = op.SetCompressors([]string{"zstd"}).SetZstdLevel(3)
		op = op.SetMaxConnIdleTime(time.Duration(mongoc.MaxIdletime))
		op = op.SetMaxPoolSize(mongoc.MaxOpen)
		op = op.SetSocketTimeout(time.Duration(mongoc.IoTimeout))
		op = op.SetHeartbeatInterval(time.Second)
		//default:secondary is preferred to be selected,if there is no secondary,primary will be selected
		op = op.SetReadPreference(readpref.SecondaryPreferred())
		//default:only read the selected server's data
		op = op.SetReadConcern(readconcern.Local())
		//default:data will be writeen to the primary's journal then return success
		op = op.SetWriteConcern(writeconcern.New(writeconcern.WMajority(), writeconcern.J(true), writeconcern.WTimeout(time.Duration(mongoc.IoTimeout))))
		tempdb, e := mongo.Connect(nil, op)
		if e != nil {
			log.Error("[config.initsource] open mongodb:", k, "error:", e)
			Close()
			os.Exit(1)
		}
		mongos[k] = tempdb
	}
	sqls = make(map[string]*sql.DB, len(sc.Sql))
	for k, sqlc := range sc.Sql {
		if k == "example_sql" {
			continue
		}
		tempdb, e := sql.Open("mysql", (&mysql.Config{
			User:                 sqlc.Username,
			Passwd:               sqlc.Passwd,
			Net:                  "tcp",
			Addr:                 sqlc.Addr,
			Timeout:              time.Duration(sqlc.ConnTimeout),
			WriteTimeout:         time.Duration(sqlc.IoTimeout),
			ReadTimeout:          time.Duration(sqlc.IoTimeout),
			AllowNativePasswords: true,
			Collation:            sqlc.Collation,
		}).FormatDSN())
		if e != nil {
			log.Error("[config.initsource] open mysql:", k, "error:", e)
			Close()
			os.Exit(1)
		}
		tempdb.SetMaxOpenConns(sqlc.MaxOpen)
		tempdb.SetConnMaxIdleTime(time.Duration(sqlc.MaxIdletime))
		sqls[k] = tempdb
	}
	rediss = make(map[string]*redis.Pool, len(sc.Redis))
	for k, redisc := range sc.Redis {
		if k == "example_redis" {
			continue
		}
		rediss[k] = redis.NewRedis(&redis.Config{
			Username:    redisc.Username,
			Password:    redisc.Passwd,
			Addr:        redisc.Addr,
			MaxOpen:     redisc.MaxOpen,
			MaxIdletime: time.Duration(redisc.MaxIdletime),
			IOTimeout:   time.Duration(redisc.IoTimeout),
			ConnTimeout: time.Duration(redisc.ConnTimeout),
		})
	}
	kafkaSubers = make(map[string]*kafka.Reader, len(sc.KafkaSub))
	for topic, subc := range sc.KafkaSub {
		if topic == "example_topic" {
			continue
		}
		dialer := &kafka.Dialer{
			Timeout:   time.Second,
			DualStack: true,
		}
		if subc.Username != "" && subc.Passwd != "" {
			var e error
			dialer.SASLMechanism, e = scram.Mechanism(scram.SHA512, subc.Username, subc.Passwd)
			if e != nil {
				log.Error("[config.initsource] kafka topic:", topic, "sub group:", subc.GroupName, "username and password parse error:", e)
				Close()
				os.Exit(1)
			}
		}
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:                []string{subc.Addr},
			Dialer:                 dialer,
			Topic:                  topic,
			GroupID:                subc.GroupName,
			StartOffset:            subc.StartOffset,
			MinBytes:               1,
			MaxBytes:               1024 * 1024 * 10,
			CommitInterval:         time.Duration(subc.CommitInterval),
			IsolationLevel:         kafka.ReadCommitted,
			HeartbeatInterval:      time.Second,
			PartitionWatchInterval: time.Second,
			WatchPartitionChanges:  true,
		})
		kafkaSubers[topic+subc.GroupName] = reader
	}
	kafkaPubers = make(map[string]*kafka.Writer, len(sc.KafkaPub))
	for topic, pubc := range sc.KafkaPub {
		if topic == "example_topic" {
			continue
		}
		dialer := &kafka.Dialer{
			Timeout:   time.Second,
			DualStack: true,
		}
		if pubc.Username != "" && pubc.Passwd != "" {
			var e error
			dialer.SASLMechanism, e = scram.Mechanism(scram.SHA512, pubc.Username, pubc.Passwd)
			if e != nil {
				log.Error("[config.initsource] kafka topic:", topic, "pub username and password parse error:", e)
				Close()
				os.Exit(1)
			}
		}
		writer := kafka.NewWriter(kafka.WriterConfig{
			Brokers:          []string{pubc.Addr},
			Topic:            topic,
			Dialer:           dialer,
			ReadTimeout:      time.Second,
			WriteTimeout:     time.Second,
			Balancer:         &kafka.Hash{},
			MaxAttempts:      3,
			RequiredAcks:     int(kafka.RequireAll),
			Async:            false,
			CompressionCodec: kafka.Snappy.Codec(),
		})
		kafkaPubers[topic] = writer
	}
}
func initapp() {
	data, e := os.ReadFile("AppConfig.json")
	if e != nil {
		log.Error("[config.initapp] read config file error:", e)
		Close()
		os.Exit(1)
	}
	AC = &AppConfig{}
	if e = json.Unmarshal(data, AC); e != nil {
		log.Error("[config.initapp] config file format error:", e)
		Close()
		os.Exit(1)
	}
	watcher, e = fsnotify.NewWatcher()
	if e != nil {
		log.Error("[config.initapp] create watcher for hot update error:", e)
		Close()
		os.Exit(1)
	}
	if e = watcher.Add("./"); e != nil {
		log.Error("[config.initapp] create watcher for hot update error:", e)
		Close()
		os.Exit(1)
	}
	closech = make(chan struct{})
	go func() {
		defer close(closech)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != "AppConfig.json" || (event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Write == 0) {
					continue
				}
				data, e := os.ReadFile("AppConfig.json")
				if e != nil {
					log.Error("[config.initapp] hot update read config file error:", e)
					continue
				}
				c := &AppConfig{}
				if e = json.Unmarshal(data, c); e != nil {
					log.Error("[config.initapp] hot update config file format error:", e)
					continue
				}
				atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&AC)), unsafe.Pointer(c))
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error("[config.initapp] hot update watcher error:", err)
			}
		}
	}()
}

//Close -
func Close() {
	if watcher != nil {
		watcher.Close()
		<-closech
	}
	log.Close()
}

//GetRpcConfig get the rpc net config
func GetRpcConfig() *RpcConfig {
	return sc.Rpc
}

//GetWebConfig get the web net config
func GetWebConfig() *WebConfig {
	return sc.Web
}

//GetMongo get a mongodb client by db's instance name
//return nil means not exist
func GetMongo(mongoname string) *mongo.Client {
	return mongos[mongoname]
}

//GetSql get a mysql db client by db's instance name
//return nil means not exist
func GetSql(mysqlname string) *sql.DB {
	return sqls[mysqlname]
}

//GetRedis get a redis client by redis's instance name
//return nil means not exist
func GetRedis(redisname string) *redis.Pool {
	return rediss[redisname]
}

//GetKafkaSuber get a kafka sub client by topic and groupid
func GetKafkaSuber(topic string, groupid string) *kafka.Reader {
	return kafkaSubers[topic+groupid]
}

//GetKafkaPuber get a kafka pub client by topic name
func GetKafkaPuber(topic string) *kafka.Writer {
	return kafkaPubers[topic]
}
`
const path = "./config/"
const name = "config.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("config").Parse(strings.ReplaceAll(text, "$", "`"))
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+name, e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+name, e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s form template error:%s", path+name, e))
	}
}
