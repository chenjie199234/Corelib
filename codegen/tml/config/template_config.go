package config

import (
	"os"
	"strings"
	"text/template"
)

const txt = `package config

import (
	"os"
	"strconv"
	"time"

	"{{.}}/model"

	"github.com/chenjie199234/Corelib/log"
	configsdk "github.com/chenjie199234/admin/sdk/config"
)

// EnvConfig can't hot update,all these data is from system env setting
// nil field means that system env not exist
type EnvConfig struct {
	ConfigType *int
	RunEnv     *string
	DeployEnv  *string
}

// EC -
var EC *EnvConfig

// RemoteConfigSdk -
var RemoteConfigSdk *configsdk.ConfigSdk

// notice is a sync function
// don't write block logic inside it
func Init(notice func(c *AppConfig)) {
	initenv()
	if EC.ConfigType != nil && *EC.ConfigType == 1 {
		tmer := time.NewTimer(time.Second * 2)
		waitapp := make(chan *struct{}, 1)
		waitsource := make(chan *struct{}, 1)
		initremoteapp(notice, waitapp)
		stopwatchsource := initremotesource(waitsource)
		appinit := false
		sourceinit := false
		for {
			select {
			case <-waitapp:
				appinit = true
			case <-waitsource:
				sourceinit = true
				stopwatchsource()
			case <-tmer.C:
				log.Error(nil, "[config.Init] timeout")
				Close()
				os.Exit(1)
			}
			if appinit && sourceinit {
				break
			}
		}
	} else {
		initlocalapp(notice)
		initlocalsource()
	}
}

// Close -
func Close() {
	log.Close()
}

func initenv() {
	EC = &EnvConfig{}
	if str, ok := os.LookupEnv("CONFIG_TYPE"); ok && str != "<CONFIG_TYPE>" && str != "" {
		configtype, e := strconv.Atoi(str)
		if e != nil || (configtype != 0 && configtype != 1 && configtype != 2) {
			log.Error(nil, "[config.initenv] env CONFIG_TYPE must be number in [0,1,2]")
			Close()
			os.Exit(1)
		}
		EC.ConfigType = &configtype
	} else {
		log.Warn(nil, "[config.initenv] missing env CONFIG_TYPE")
	}
	if EC.ConfigType != nil && *EC.ConfigType == 1 {
		var e error
		if RemoteConfigSdk, e = configsdk.NewConfigSdk(model.Project, model.Group, model.Name, nil); e != nil {
			log.Error(nil, "[config.initenv] new remote config sdk failed", log.CError(e))
			Close()
			os.Exit(1)
		}
	}
	if str, ok := os.LookupEnv("RUN_ENV"); ok && str != "<RUN_ENV>" && str != "" {
		EC.RunEnv = &str
	} else {
		log.Warn(nil, "[config.initenv] missing env RUN_ENV")
	}
	if str, ok := os.LookupEnv("DEPLOY_ENV"); ok && str != "<DEPLOY_ENV>" && str != "" {
		EC.DeployEnv = &str
	} else {
		log.Warn(nil, "[config.initenv] missing env DEPLOY_ENV")
	}
}`
const apptxt = `package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/chenjie199234/Corelib/log"
	publicmids "github.com/chenjie199234/Corelib/mids"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/fsnotify/fsnotify"
)

// AppConfig can hot update
// this is the config used for this app
type AppConfig struct {
	HandlerTimeout     map[string]map[string]ctime.Duration $json:"handler_timeout"$      //first key path,second key method(GET,POST,PUT,PATCH,DELETE,CRPC,GRPC),value timeout
	WebPathRewrite     map[string]map[string]string         $json:"web_path_rewrite"$     //first key method(GET,POST,PUT,PATCH,DELETE),second key origin url,value new url
	HandlerRate        publicmids.MultiPathRateConfigs      $json:"handler_rate"$         //key:path
	Accesses           publicmids.MultiPathAccessConfigs    $json:"accesses"$             //key:path
	TokenSecret        string                               $json:"token_secret"$         //if don't need token check,this can be ingored
	SessionTokenExpire ctime.Duration                       $json:"session_token_expire"$ //if don't need session and token check,this can be ignored
	Service            *ServiceConfig                       $json:"service"$
}
type ServiceConfig struct {
	//add your config here
}

// every time update AppConfig will call this function
func validateAppConfig(ac *AppConfig) {
}

// AC -
var AC *AppConfig

var watcher *fsnotify.Watcher

func initlocalapp(notice func(*AppConfig)) {
	data, e := os.ReadFile("./AppConfig.json")
	if e != nil {
		log.Error(nil, "[config.local.app] read config file failed", log.CError(e))
		Close()
		os.Exit(1)
	}
	AC = &AppConfig{}
	if e = json.Unmarshal(data, AC); e != nil {
		log.Error(nil, "[config.local.app] config file format wrong", log.CError(e))
		Close()
		os.Exit(1)
	}
	validateAppConfig(AC)
	log.Info(nil, "[config.local.app] update success", log.Any("config", AC))
	if notice != nil {
		notice(AC)
	}
	watcher, e = fsnotify.NewWatcher()
	if e != nil {
		log.Error(nil, "[config.local.app] create watcher for hot update failed", log.CError(e))
		Close()
		os.Exit(1)
	}
	if e = watcher.Add("./"); e != nil {
		log.Error(nil, "[config.local.app] create watcher for hot update failed", log.CError(e))
		Close()
		os.Exit(1)
	}
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != "AppConfig.json" || (!event.Has(fsnotify.Create) && !event.Has(fsnotify.Write)) {
					continue
				}
				data, e := os.ReadFile("./AppConfig.json")
				if e != nil {
					log.Error(nil, "[config.local.app] hot update read config file failed", log.CError(e))
					continue
				}
				c := &AppConfig{}
				if e = json.Unmarshal(data, c); e != nil {
					log.Error(nil, "[config.local.app] hot update config file format wrong", log.CError(e))
					continue
				}
				validateAppConfig(c)
				AC = c
				log.Info(nil, "[config.local.app] update success", log.Any("config", AC))
				if notice != nil {
					notice(AC)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error(nil, "[config.local.app] hot update watcher failed", log.CError(err))
			}
		}
	}()
}
func initremoteapp(notice func(*AppConfig), wait chan *struct{}) (stopwatch func()) {
	return RemoteConfigSdk.Watch("AppConfig", func(key, keyvalue, keytype string) {
		//only support json
		if keytype != "json" {
			log.Error(nil, "[config.remote.app] config data can only support json format")
			return
		}
		c := &AppConfig{}
		if e := json.Unmarshal(common.STB(keyvalue), c); e != nil {
			log.Error(nil, "[config.remote.app] config data format wrong", log.CError(e))
			return
		}
		validateAppConfig(c)
		AC = c
		log.Info(nil, "[config.remote.app] update success", log.Any("config", AC))
		if notice != nil {
			notice(AC)
		}
		select {
		case wait <- nil:
		default:
		}
	})
}`
const sourcetxt = `package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/mongo"
	"github.com/chenjie199234/Corelib/mysql"
	"github.com/chenjie199234/Corelib/redis"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/web"
)

// sourceConfig can't hot update
type sourceConfig struct {
	CGrpcServer *CGrpcServerConfig      $json:"cgrpc_server"$
	CGrpcClient *CGrpcClientConfig      $json:"cgrpc_client"$
	CrpcServer  *CrpcServerConfig       $json:"crpc_server"$
	CrpcClient  *CrpcClientConfig       $json:"crpc_client"$
	WebServer   *WebServerConfig        $json:"web_server"$
	WebClient   *WebClientConfig        $json:"web_client"$
	Mongo       map[string]*MongoConfig $json:"mongo"$ //key example:xx_mongo
	Mysql       map[string]*MysqlConfig $json:"mysql"$ //key example:xx_mysql
	Redis       map[string]*RedisConfig $json:"redis"$ //key example:xx_redis
}

// CGrpcServerConfig
type CGrpcServerConfig struct {
	Certs map[string]string $json:"certs"$ //key cert path,value private key path,if this is not empty,tls will be used
	*cgrpc.ServerConfig
}

// CGrpcClientConfig
type CGrpcClientConfig struct {
	*cgrpc.ClientConfig
}

// CrpcServerConfig -
type CrpcServerConfig struct {
	Certs map[string]string $json:"certs"$ //key cert path,value private key path,if this is not empty,tls will be used
	*crpc.ServerConfig
}

// CrpcClientConfig -
type CrpcClientConfig struct {
	*crpc.ClientConfig
}

// WebServerConfig -
type WebServerConfig struct {
	Certs map[string]string $json:"certs"$ //key cert path,value private key path,if this is not empty,tls will be used
	*web.ServerConfig
}

// WebClientConfig -
type WebClientConfig struct {
	*web.ClientConfig
}

// RedisConfig -
type RedisConfig struct {
	TLS             bool     $json:"tls"$
	SpecificCAPaths []string $json:"specific_ca_paths"$ //only when TLS is true,this will be effective,if this is empty,system's ca will be used
	*redis.Config
}

// MysqlConfig -
type MysqlConfig struct {
	TLS             bool     $json:"tls"$
	SpecificCAPaths []string $json:"specific_ca_paths"$ //only when TLS is true,this will be effective,if this is empty,system's ca will be used
	*mysql.Config
}

// MongoConfig -
type MongoConfig struct {
	TLS             bool     $json:"tls"$
	SpecificCAPaths []string $json:"specific_ca_paths"$ //only when TLS is true,this will be effective,if this is empty,system's ca will be used
	*mongo.Config
}

// SC total source config instance
var sc *sourceConfig

var mongos map[string]*mongo.Client

var mysqls map[string]*mysql.Client

var rediss map[string]*redis.Client

func initlocalsource() {
	data, e := os.ReadFile("./SourceConfig.json")
	if e != nil {
		log.Error(nil, "[config.local.source] read config file failed", log.CError(e))
		Close()
		os.Exit(1)
	}
	sc = &sourceConfig{}
	if e = json.Unmarshal(data, sc); e != nil {
		log.Error(nil, "[config.local.source] config file format wrong", log.CError(e))
		Close()
		os.Exit(1)
	}
	log.Info(nil, "[config.local.source] update success", log.Any("config", sc))
	initsource()
}
func initremotesource(wait chan *struct{}) (stopwatch func()) {
	return RemoteConfigSdk.Watch("SourceConfig", func(key, keyvalue, keytype string) {
		//only support json
		if keytype != "json" {
			log.Error(nil, "[config.remote.source] config data can only support json format")
			return
		}
		//source config only init once
		if sc != nil {
			return
		}
		c := &sourceConfig{}
		if e := json.Unmarshal(common.STB(keyvalue), c); e != nil {
			log.Error(nil, "[config.remote.source] config data format wrong", log.CError(e))
			return
		}
		sc = c
		log.Info(nil, "[config.remote.source] update success", log.Any("config", sc))
		initsource()
		select {
		case wait <- nil:
		default:
		}
	})
}
func initsource() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		initgrpcserver()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initgrpcclient()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initcrpcserver()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initcrpcclient()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initwebserver()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initwebclient()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initredis()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initmongo()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		initmysql()
		wg.Done()
	}()
	wg.Wait()
}
func initgrpcserver() {
	if sc.CGrpcServer == nil {
		sc.CGrpcServer = &CGrpcServerConfig{
			ServerConfig: &cgrpc.ServerConfig{
				ConnectTimeout: ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:  ctime.Duration(time.Millisecond * 500),
				HeartProbe:     ctime.Duration(time.Second * 5),
				IdleTimeout:    0,
			},
		}
	} else {
		if sc.CGrpcServer.ConnectTimeout <= 0 {
			sc.CGrpcServer.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CGrpcServer.GlobalTimeout <= 0 {
			sc.CGrpcServer.GlobalTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CGrpcServer.HeartProbe <= 0 {
			sc.CGrpcServer.HeartProbe = ctime.Duration(time.Second * 5)
		}
	}
}
func initgrpcclient() {
	if sc.CGrpcClient == nil {
		sc.CGrpcClient = &CGrpcClientConfig{
			ClientConfig: &cgrpc.ClientConfig{
				ConnectTimeout: ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:  ctime.Duration(time.Millisecond * 500),
				HeartProbe:     ctime.Duration(time.Second* 5),
				IdleTimeout:    0,
			},
		}
	} else {
		if sc.CGrpcClient.ConnectTimeout <= 0 {
			sc.CGrpcClient.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CGrpcClient.GlobalTimeout < 0 {
			sc.CGrpcClient.GlobalTimeout = 0
		}
		if sc.CGrpcClient.HeartProbe <= 0 {
			sc.CGrpcClient.HeartProbe = ctime.Duration(time.Second * 5)
		}
	}
}
func initcrpcserver() {
	if sc.CrpcServer == nil {
		sc.CrpcServer = &CrpcServerConfig{
			ServerConfig: &crpc.ServerConfig{
				ConnectTimeout: ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:  ctime.Duration(time.Millisecond * 500),
				HeartProbe:     ctime.Duration(time.Second * 5),
				IdleTimeout:    0,
			},
		}
	} else {
		if sc.CrpcServer.ConnectTimeout <= 0 {
			sc.CrpcServer.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CrpcServer.GlobalTimeout <= 0 {
			sc.CrpcServer.GlobalTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CrpcServer.HeartProbe <= 0 {
			sc.CrpcServer.HeartProbe = ctime.Duration(time.Second * 5)
		}
	}
}
func initcrpcclient() {
	if sc.CrpcClient == nil {
		sc.CrpcClient = &CrpcClientConfig{
			ClientConfig: &crpc.ClientConfig{
				ConnectTimeout: ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:  ctime.Duration(time.Millisecond * 500),
				HeartProbe:     ctime.Duration(time.Second * 5),
				IdleTimeout:    0,
			},
		}
	} else {
		if sc.CrpcClient.ConnectTimeout <= 0 {
			sc.CrpcClient.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.CrpcClient.GlobalTimeout < 0 {
			sc.CrpcClient.GlobalTimeout = 0
		}
		if sc.CrpcClient.HeartProbe <= 0 {
			sc.CrpcClient.HeartProbe = ctime.Duration(time.Second * 5)
		}
	}

}
func initwebserver() {
	if sc.WebServer == nil {
		sc.WebServer = &WebServerConfig{
			ServerConfig: &web.ServerConfig{
				WaitCloseMode:        0,
				WaitCloseTime:        ctime.Duration(time.Second),
				ConnectTimeout:       ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:        ctime.Duration(time.Millisecond * 500),
				IdleTimeout:          ctime.Duration(time.Second * 5),
				MaxRequestHeader:     2048,
				CorsAllowedOrigins:   []string{"*"},
				CorsAllowedHeaders:   []string{"*"},
				CorsExposeHeaders:    []string{"*"},
				CorsAllowCredentials: false,
				CorsMaxAge:           ctime.Duration(time.Minute * 30),
				SrcRootPath:          "./src",
			},
		}
	} else {
		if sc.WebServer.WaitCloseMode != 0 && sc.WebServer.WaitCloseMode != 1 {
			log.Error(nil, "[config.initwebserver] wait_close_mode must be 0 or 1")
			Close()
			os.Exit(1)
		}
		if sc.WebServer.ConnectTimeout <= 0 {
			sc.WebServer.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.WebServer.GlobalTimeout <= 0 {
			sc.WebServer.GlobalTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.WebServer.IdleTimeout <= 0 {
			sc.WebServer.IdleTimeout = ctime.Duration(time.Second * 5)
		}
	}
}
func initwebclient() {
	if sc.WebClient == nil {
		sc.WebClient = &WebClientConfig{
			ClientConfig: &web.ClientConfig{
				ConnectTimeout:    ctime.Duration(time.Millisecond * 500),
				GlobalTimeout:     ctime.Duration(time.Millisecond * 500),
				IdleTimeout:       ctime.Duration(time.Second * 5),
				MaxResponseHeader: 4096,
			},
		}
	} else {
		if sc.WebClient.ConnectTimeout <= 0 {
			sc.WebClient.ConnectTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if sc.WebClient.GlobalTimeout < 0 {
			sc.WebClient.GlobalTimeout = 0
		}
		if sc.WebClient.IdleTimeout <= 0 {
			sc.WebClient.IdleTimeout = ctime.Duration(time.Second * 5)
		}
	}
}
func initredis(){
	for k, redisc := range sc.Redis {
		if k == "example_redis" {
			continue
		}
		redisc.RedisName = k
		if len(redisc.Addrs) == 0 {
			redisc.Addrs = []string{"127.0.0.1:6379"}
		}
		if redisc.MaxConnIdletime <= 0 {
			redisc.MaxConnIdletime = ctime.Duration(time.Minute * 5)
		}
		if redisc.IOTimeout <= 0 {
			redisc.IOTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if redisc.DialTimeout <= 0 {
			redisc.DialTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	rediss = make(map[string]*redis.Client, len(sc.Redis))
	lker := sync.Mutex{}
	wg := sync.WaitGroup{}
	for k, v := range sc.Redis {
		if k == "example_redis" {
			continue
		}
		redisc := v
		wg.Add(1)
		go func(){
			defer wg.Done()
			var tlsc *tls.Config
			if redisc.TLS {
				tlsc = &tls.Config{}
				if len(redisc.SpecificCAPaths) > 0 {
					tlsc.RootCAs = x509.NewCertPool()
					for _, certpath := range redisc.SpecificCAPaths {
						cert, e := os.ReadFile(certpath)
						if e != nil {
							log.Error(nil, "[config.initredis] read specific cert failed",
								log.String("redis", redisc.RedisName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
						if ok := tlsc.RootCAs.AppendCertsFromPEM(cert); !ok {
							log.Error(nil, "[config.initredis] specific cert load failed",
								log.String("redis", redisc.RedisName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
					}
				}
			}
			c, e := redis.NewRedis(redisc.Config, tlsc)
			if e != nil {
				log.Error(nil, "[config.initredis] failed",
					log.String("redis", redisc.RedisName), log.CError(e))
				Close()
				os.Exit(1)
			}
			lker.Lock()
			rediss[redisc.RedisName] = c
			lker.Unlock()
		}()
	}
	wg.Wait()
}
func initmongo(){
	for k, mongoc := range sc.Mongo {
		if k == "example_mongo" {
			continue
		}
		mongoc.MongoName = k
		if len(mongoc.Addrs) == 0 {
			mongoc.Addrs = []string{"127.0.0.1:27017"}
		}
		if mongoc.MaxConnIdletime <= 0 {
			mongoc.MaxConnIdletime = ctime.Duration(time.Minute * 5)
		}
		if mongoc.IOTimeout <= 0 {
			mongoc.IOTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if mongoc.DialTimeout <= 0 {
			mongoc.DialTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	mongos = make(map[string]*mongo.Client, len(sc.Mongo))
	lker := sync.Mutex{}
	wg := sync.WaitGroup{}
	for k, v := range sc.Mongo {
		if k == "example_mongo" {
			continue
		}
		mongoc := v
		wg.Add(1)
		go func(){
			defer wg.Done()
			var tlsc *tls.Config
			if mongoc.TLS {
				tlsc = &tls.Config{}
				if len(mongoc.SpecificCAPaths) > 0 {
					tlsc.RootCAs = x509.NewCertPool()
					for _, certpath := range mongoc.SpecificCAPaths {
						cert, e := os.ReadFile(certpath)
						if e != nil {
							log.Error(nil, "[config.initmongo] read specific cert failed",
								log.String("mongo", mongoc.MongoName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
						if ok := tlsc.RootCAs.AppendCertsFromPEM(cert); !ok {
							log.Error(nil, "[config.initmongo] specific cert load failed",
								log.String("mongo", mongoc.MongoName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
					}
				}
			}
			c, e := mongo.NewMongo(mongoc.Config, tlsc)
			if e != nil {
				log.Error(nil, "[config.initmongo] failed", log.String("mongo", mongoc.MongoName), log.CError(e))
				Close()
				os.Exit(1)
			}
			lker.Lock()
			mongos[mongoc.MongoName] = c
			lker.Unlock()
		}()
	}
	wg.Wait()
}
func initmysql(){
	for k, mysqlc := range sc.Mysql {
		if k == "example_mysql" {
			continue
		}
		mysqlc.MysqlName = k
		if mysqlc.MaxConnIdletime <= 0 {
			mysqlc.MaxConnIdletime = ctime.Duration(time.Minute * 5)
		}
		if mysqlc.IOTimeout <= 0 {
			mysqlc.IOTimeout = ctime.Duration(time.Millisecond * 500)
		}
		if mysqlc.DialTimeout <= 0 {
			mysqlc.DialTimeout = ctime.Duration(time.Millisecond * 250)
		}
	}
	mysqls = make(map[string]*mysql.Client, len(sc.Mysql))
	lker := sync.Mutex{}
	wg := sync.WaitGroup{}
	for k, v := range sc.Mysql {
		if k == "example_mysql" {
			continue
		}
		mysqlc := v
		wg.Add(1)
		go func(){
			defer wg.Done()
			var tlsc *tls.Config
			if mysqlc.TLS {
				tlsc = &tls.Config{}
				if len(mysqlc.SpecificCAPaths) > 0 {
					tlsc.RootCAs = x509.NewCertPool()
					for _, certpath := range mysqlc.SpecificCAPaths {
						cert, e := os.ReadFile(certpath)
						if e != nil {
							log.Error(nil, "[config.initmysql] read specific cert failed",
								log.String("mysql", mysqlc.MysqlName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
						if ok := tlsc.RootCAs.AppendCertsFromPEM(cert); !ok {
							log.Error(nil, "[config.initmysql] specific cert load failed",
								log.String("mysql", mysqlc.MysqlName), log.String("cert_path", certpath), log.CError(e))
							Close()
							os.Exit(1)
						}
					}
				}
			}
			c, e := mysql.NewMysql(mysqlc.Config, tlsc)
			if e != nil {
				log.Error(nil, "[config.initmysql] failed", log.String("mysql", mysqlc.MysqlName), log.CError(e))
				Close()
				os.Exit(1)
			}
			lker.Lock()
			mysqls[mysqlc.MysqlName] = c
			lker.Unlock()
		}()
	}
	wg.Wait()
}

// GetCGrpcServerConfig get the grpc net config
func GetCGrpcServerConfig() *CGrpcServerConfig {
	return sc.CGrpcServer
}

// GetCGrpcClientConfig get the grpc net config
func GetCGrpcClientConfig() *CGrpcClientConfig {
	return sc.CGrpcClient
}

// GetCrpcServerConfig get the crpc net config
func GetCrpcServerConfig() *CrpcServerConfig {
	return sc.CrpcServer
}

// GetCrpcClientConfig get the crpc net config
func GetCrpcClientConfig() *CrpcClientConfig {
	return sc.CrpcClient
}

// GetWebServerConfig get the web net config
func GetWebServerConfig() *WebServerConfig {
	return sc.WebServer
}

// GetWebClientConfig get the web net config
func GetWebClientConfig() *WebClientConfig {
	return sc.WebClient
}

// GetMongo get a mongodb client by db's instance name
// return nil means not exist
func GetMongo(mongoname string) *mongo.Client {
	return mongos[mongoname]
}

// GetMysql get a mysql db client by db's instance name
// return nil means not exist
func GetMysql(mysqlname string) *mysql.Client {
	return mysqls[mysqlname]
}

// GetRedis get a redis client by redis's instance name
// return nil means not exist
func GetRedis(redisname string) *redis.Client {
	return rediss[redisname]
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./config/", 0755); e != nil {
		panic("mkdir ./config/ error: " + e.Error())
	}
	//./config/config.go
	configtemplate, e := template.New("./config/config.go").Parse(txt)
	if e != nil {
		panic("parse ./config/config.go template error: " + e.Error())
	}
	configfile, e := os.OpenFile("./config/config.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./config/config.go error: " + e.Error())
	}
	if e := configtemplate.Execute(configfile, packagename); e != nil {
		panic("write ./config/config.go error: " + e.Error())
	}
	if e := configfile.Sync(); e != nil {
		panic("sync ./config/config.go error: " + e.Error())
	}
	if e := configfile.Close(); e != nil {
		panic("close ./config/config.go error: " + e.Error())
	}
	//./config/app_config.go
	appfile, e := os.OpenFile("./config/app_config.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./config/app_config.go error: " + e.Error())
	}
	if _, e := appfile.WriteString(strings.ReplaceAll(apptxt, "$", "`")); e != nil {
		panic("write ./config/app_config.go error: " + e.Error())
	}
	if e := appfile.Sync(); e != nil {
		panic("sync ./config/app_config.go error: " + e.Error())
	}
	if e := appfile.Close(); e != nil {
		panic("close ./config/app_config.go error: " + e.Error())
	}
	//./config/source_config.go
	sourcefile, e := os.OpenFile("./config/source_config.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./config/source_config.go error: " + e.Error())
	}
	if _, e := sourcefile.WriteString(strings.ReplaceAll(sourcetxt, "$", "`")); e != nil {
		panic("write ./config/source_config.go error: " + e.Error())
	}
	if e := sourcefile.Sync(); e != nil {
		panic("sync ./config/source_config.go error: " + e.Error())
	}
	if e := sourcefile.Close(); e != nil {
		panic("close ./config/source_config.go error: " + e.Error())
	}
}
