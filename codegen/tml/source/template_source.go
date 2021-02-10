package source

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

const text = `package source

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-sql-driver/mysql"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"github.com/chenjie199234/Corelib/log"
)

//XDuration transfer to time.Duration by time.Duration(XDuration)
type XDuration time.Duration

//UnmarshalText -
func (d *XDuration) UnmarshalJSON(data []byte) error {
	if data[0] == '"' && data[len(data)-1] == '"' {
		if len(data) == 2 {
			*d = XDuration(0)
			return nil
		}
		data = data[1 : len(data)-1]
	}
	if data[0] != '"' && data[len(data)-1] != '"' {
		temp, e := time.ParseDuration(string(data))
		if e != nil {
			return fmt.Errorf("format wrong for XDuration")
		}
		*d = XDuration(temp)
	} else {
		return fmt.Errorf("format wrong for XDuration")
	}
	return nil
}

//SourceConfig can't hot update
type sourceConfig struct {
	Log      *LogConfig                 $json:"log"$
	Rpc      *RpcConfig                 $json:"rpc"$
	Http     *HttpConfig                $json:"http"$
	DB       map[string]*DBConfig       $json:"db"$        //key xx_db
	Redis    map[string]*RedisConfig    $json:"redis"$     //key xx_redis
	KafkaPub map[string]*KafkaPubConfig $json:"kafka_pub"$ //key topic name
	KafkaSub map[string]*KafkaSubConfig $json:"kafka_sub"$ //key topic name
}

//LogConfig -
type LogConfig struct {
	LogPath   string $json:"log_path"$
	Debug     bool   $json:"debug"$
	MultiFile bool   $json:"multi_file"$
}

//RpcConfig -
type RpcConfig struct {
	RpcPort    uint      $json:"rpc_port"$
	RpcTimeout XDuration $json:"rpc_timeout"$ //default 500ms
}

//HttpConfig -
type HttpConfig struct {
	//server
	HttpPort     uint      $json:"http_port"$
	HttpTimeout  XDuration $json:"http_timeout"$ //default 500ms
	HttpCertFile string    $json:"http_certfile"$
	HttpKeyFile  string    $json:"http_keyfile"$
	//cors
	HttpCors *HttpCorsConfig $json:"http_cors"$
}

//HttpCorsConfig -
type HttpCorsConfig struct {
	CorsOrigin []string $json:"cors_origin"$
	CorsMethod []string $json:"cors_method"$
	CorsHeader []string $json:"cors_header"$
	CorsExpose []string $json:"cors_expose"$
}

//RedisConfig -
type RedisConfig struct {
	Username    string    $json:"username"$
	Passwd      string    $json:"passwd"$
	Net         string    $json:"net"$
	Addr        string    $json:"addr"$
	Maxopen     int       $json:"max_open"$     //default 256 //max num of connections can be opened
	MaxIdletime XDuration $json:"max_idletime"$ //default 1min //max time a connection can be idle,more then this time,connection will be closed
	IoTimeout   XDuration $json:"io_timeout"$   //default 500ms
	ConnTimeout XDuration $json:"conn_timeout"$ //default 250ms
}

//DBConfig -
type DBConfig struct {
	Username    string    $json:"username"$
	Passwd      string    $json:"passwd"$
	Net         string    $json:"net"$
	Addr        string    $json:"addr"$
	Collation   string    $json:"collation"$
	Maxopen     int       $json:"max_open"$     //default 256 //max num of connections can be opened
	MaxIdletime XDuration $json:"max_idletime"$ //default 1min //max time a connection can be idle,more then this time,connection will be closed
	IoTimeout   XDuration $json:"io_timeout"$   //default 500ms
	ConnTimeout XDuration $json:"conn_timeout"$ //default 250ms
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
	CommitInterval XDuration $json:"commit_interval"$
}

//SC total source config instance
var sc *sourceConfig

var dbs map[string]*sql.DB

var caches map[string]*redis.Client

var kafkaSubers map[string]*kafka.Reader

var kafkaPubers map[string]*kafka.Writer

func init() {
	data, e := ioutil.ReadFile("SourceConfig.json")
	if e != nil {
		panic("[SourceConfig]read config file error:" + e.Error())
	}
	sc = &sourceConfig{}
	if e = json.Unmarshal(data, sc); e != nil {
		panic(fmt.Sprintf("[SourceConfig]format error:%s", e))
	}
	sc.validate()
	log.Init(&log.Config{
		LogPath:   sc.Log.LogPath,
		Debug:     sc.Log.Debug,
		MultiFile: sc.Log.MultiFile,
		AppName:   "{{.}}",
	})
	sc.newsource()
}

func (c *sourceConfig) validate() {
	if c.Log.LogPath == "" {
		panic("[SourceConfig]log path is empty")
	}
	if c.Rpc.RpcPort == 0 {
		//use default
		c.Rpc.RpcPort = 9000
	}
	if c.Rpc.RpcTimeout == 0 {
		//use default
		c.Rpc.RpcTimeout = XDuration(time.Millisecond * 500)
	}
	if c.Http.HttpPort == 0 {
		//use default
		c.Http.HttpPort = 8000
	}
	if c.Http.HttpTimeout == 0 {
		//use default
		c.Http.HttpTimeout = XDuration(time.Millisecond * 500)
	}
	for _, dbc := range c.DB {
		//mysql can don't use passwd but must use username
		if dbc.Username == "" {
			dbc.Username = "root"
			dbc.Passwd = "root"
		}
		if dbc.Addr == "" || dbc.Net == "" {
			dbc.Addr = "127.0.0.1:3306"
			dbc.Net = "tcp"
		}
		if dbc.Maxopen == 0 {
			//use default
			dbc.Maxopen = 100
		}
		if dbc.MaxIdletime == 0 {
			//use default
			dbc.MaxIdletime = XDuration(time.Minute * 10)
		}
		if dbc.IoTimeout == 0 {
			//use default
			dbc.IoTimeout = XDuration(time.Millisecond * 500)
		}
		if dbc.ConnTimeout == 0 {
			//use default
			dbc.ConnTimeout = XDuration(time.Millisecond * 250)
		}
	}
	for _, redisc := range c.Redis {
		//redis can don't use username and passwd
		if redisc.Addr == "" || redisc.Net == "" {
			redisc.Addr = "127.0.0.1:6379"
			redisc.Net = "tcp"
		}
		if redisc.Maxopen == 0 {
			//use default
			redisc.Maxopen = 100
		}
		if redisc.MaxIdletime == 0 {
			//use default
			redisc.MaxIdletime = XDuration(time.Minute * 10)
		}
		if redisc.IoTimeout == 0 {
			//use default
			redisc.IoTimeout = XDuration(time.Millisecond * 500)
		}
		if redisc.ConnTimeout == 0 {
			//use default
			redisc.ConnTimeout = XDuration(time.Millisecond * 250)
		}
	}
	for _, pubc := range c.KafkaPub {
		if pubc.Addr == "" {
			pubc.Addr = "127.0.0.1:9092"
		}
		if pubc.Username == "" && pubc.Passwd == "" {
			pubc.Username = "root"
			pubc.Passwd = "root"
		}
	}
	for topic, subc := range c.KafkaSub {
		if subc.Addr == "" {
			subc.Addr = "127.0.0.1:9092"
		}
		if subc.Username == "" && subc.Passwd == "" {
			subc.Username = "root"
			subc.Passwd = "root"
		}
		if subc.GroupName == "" {
			panic(fmt.Sprintf("[SourceConfig]sub topic:%s groupname missing", topic))
		}
	}
}

//don't change this function only if you known what you are changing
func (c *sourceConfig) newsource() {
	dbs = make(map[string]*sql.DB, len(c.DB))
	for k, dbc := range c.DB {
		if k == "example_db" {
			continue
		}
		op := &mysql.Config{}
		op.User = dbc.Username
		op.Passwd = dbc.Passwd
		op.Net = dbc.Net
		op.Addr = dbc.Addr
		op.Timeout = time.Duration(dbc.ConnTimeout)
		op.WriteTimeout = time.Duration(dbc.IoTimeout)
		op.ReadTimeout = time.Duration(dbc.IoTimeout)
		op.AllowNativePasswords = true
		op.Collation = dbc.Collation
		op.ParseTime = true
		tempdb, _ := sql.Open("mysql", op.FormatDSN())
		tempdb.SetMaxOpenConns(dbc.Maxopen)
		tempdb.SetConnMaxIdleTime(time.Duration(dbc.MaxIdletime))
		dbs[k] = tempdb
	}
	caches = make(map[string]*redis.Client, len(c.Redis))
	for k, redisc := range c.Redis {
		if k == "example_redis" {
			continue
		}
		op := &redis.Options{}
		op.Username = redisc.Username
		op.Password = redisc.Passwd
		op.Network = redisc.Net
		op.Addr = redisc.Addr
		op.PoolSize = redisc.Maxopen
		op.IdleTimeout = time.Duration(redisc.MaxIdletime)
		op.DialTimeout = time.Duration(redisc.ConnTimeout)
		op.ReadTimeout = time.Duration(redisc.IoTimeout)
		op.WriteTimeout = time.Duration(redisc.IoTimeout)
		tempredis := redis.NewClient(op)
		caches[k] = tempredis
	}
	kafkaSubers = make(map[string]*kafka.Reader, len(c.KafkaSub))
	for topic, subc := range c.KafkaSub {
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
				log.Fatalf("[source]kafka topic:%s sub group:%s username and password parse error:%s", topic, subc.GroupName, e)
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
	kafkaPubers = make(map[string]*kafka.Writer, len(c.KafkaPub))
	for topic, pubc := range c.KafkaPub {
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
				log.Fatalf("[source]kafka topic:%s pub username and password parse error:%s", topic, e)
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

//GetRpcConfig get the rpc net config
func GetRpcConfig() *RpcConfig {
	return sc.Rpc
}

//GetHttpConfig get the http net config
func GetHttpConfig() *HttpConfig {
	return sc.Http
}

//GetDB get a db client by db's logic name
//return nil means not exist
func GetDB(dbname string) *sql.DB {
	return dbs[dbname]
}

//GetRedis get a redis client by redis's logic name
//return nil means not exist
func GetRedis(redisname string) *redis.Client {
	return caches[redisname]
}

//GetKafkaSuber get a kafka sub client by topic and groupid
func GetKafkaSuber(topic string, groupid string) *kafka.Reader {
	return kafkaSubers[topic+groupid]
}

//GetKafkaPuber get a kafka pub client by topic name
func GetKafkaPuber(topic string) *kafka.Writer {
	return kafkaPubers[topic]
}`

const path = "./source/"
const name = "source.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("source").Parse(strings.ReplaceAll(text, "$", "`"))
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
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+name, e))
	}
}
