package configfile

import (
	"fmt"
	"os"
	"text/template"
)

const textsource = `{
	"cgrpc_server":{
		"connect_timeout":"200ms",
		"global_timeout":"200ms",
		"heart_probe":"1.5s"
	},
	"cgrpc_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"heart_probe":"1.5s"
	},
	"crpc_server":{
		"connect_timeout":"200ms",
		"global_timeout":"200ms",
		"heart_probe":"1.5s"
	},
	"crpc_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"heart_probe":"1.5s"
	},
	"web_server":{
		"connect_timeout":"200ms",
		"global_timeout":"200ms",
		"idle_timeout":"5s",
		"heart_probe":"1.5s",
		"static_file":"./src",
		"web_cors":{
			"cors_origin":["*"],
			"cors_header":["*"],
			"cors_expose":[]
		}
	},
	"web_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"idle_timeout":"5s",
		"heart_probe":"1.5s"
	},
	"mongo":{
		"example_mongo":{
			"url":"[mongodb/mongodb+srv]://[username:password@]host1,...,hostN[/dbname][?param1=value1&...&paramN=valueN]",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"500ms",
			"conn_timeout":"500ms"
		}
	},
	"sql":{
		"example_sql":{
			"url":"[username:password@][protocol(address)][/dbname][?param1=value1&...&paramN=valueN]",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"200ms",
			"conn_timeout":"200ms"
		}
	},
	"redis":{
		"example_redis":{
			"url":"[redis/rediss]://[[username:]password@]host[/dbindex]",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"200ms",
			"conn_timeout":"200ms"
		}
	},
	"kafka_pub":[
		{
			"addrs":["127.0.0.1:12345"],
			"username":"example",
			"password":"example",
			"auth_method":3,
			"compress_method":2,
			"topic_name":"example_topic",
			"io_timeout":"500ms",
			"conn_timeout":"200ms"
		}
	],
	"kafka_sub":[
		{
			"addrs":["127.0.0.1:12345"],
			"username":"example",
			"password":"example",
			"auth_method":3,
			"topic_name":"example_topic",
			"group_name":"example_group",
			"conn_timeout":"200ms",
			"start_offset":-2,
			"commit_interval":"0s"
		}
	]
}`
const textapp = `{
	"handler_timeout":{
		"/{{.}}.status/ping":{
			"GET":"200ms",
			"CRPC":"200ms",
			"GRPC":"200ms"
		}
	},
	"handler_rate":[{
		"Path":"/{{.}}.status/ping",
		"Method":["GET","GRPC","CRPC"],
		"MaxPerSec":10
	}],
	"access_sign_sec_keys":{
		"default":"default_sec_key",
		"/{{.}}.status/ping":"specific_sec_key"
	},
	"access_key_sec_keys":{
		"default":"default_sec_key",
		"/{{.}}.status/ping":"specific_sec_key"
	},
	"service":{

	}
}`

const path = "./"
const sourcename = "SourceConfig.json"
const appname = "AppConfig.json"

var tmlsource *template.Template
var tmlapp *template.Template

var filesource *os.File
var fileapp *os.File

func init() {
	var e error
	tmlsource, e = template.New("source").Parse(textsource)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
	tmlapp, e = template.New("app").Parse(textapp)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	filesource, e = os.OpenFile(path+sourcename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sourcename, e))
	}
	fileapp, e = os.OpenFile(path+appname, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+appname, e))
	}
}
func Execute(projectname string) {
	if e := tmlsource.Execute(filesource, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+sourcename, e))
	}
	if e := tmlapp.Execute(fileapp, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+appname, e))
	}
}
