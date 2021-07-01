package configfile

import (
	"fmt"
	"os"
	"text/template"
)

const textsource = `{
	"rpc_server":{
		"global_timeout":"200ms",
		"heart_timeout":"5s",
		"heart_probe":"1.5s"
	},
	"rpc_client":{
		"conn_timeout":"200ms",
		"global_timeout":"200ms",
		"heart_timeout":"5s",
		"heart_probe":"1.5s"
	},
	"web_server":{
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
		"global_timeout":"200ms",
		"idle_timeout":"5s",
		"heart_probe":"1.5s",
		"skip_verify_tls":false,
		"cas":["path_to_example_ca"]
	},
	"mongo":{
		"example_mongo":{
			"username":"",
			"passwd":"",
			"addr":[],
			"replica_set_name":"",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"500ms",
			"conn_timeout":"500ms"
		}
	},
	"sql":{
		"example_sql":{
			"username":"root",
			"passwd":"",
			"net":"tcp",
			"addr":"127.0.0.1:3306",
			"collation":"utf8mb4",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"200ms",
			"conn_timeout":"200ms"
		}
	},
	"redis":{
		"example_redis":{
			"username":"",
			"passwd":"",
			"net":"tcp",
			"addr":"127.0.0.1:6379",
			"max_open":100,
			"max_idletime":"10m",
			"io_timeout":"200ms",
			"conn_timeout":"200ms"
		}
	},
	"kafka_pub":{
		"example_topic":{
			"addr":"127.0.0.1:12345",
			"username":"example",
			"password":"example"
		}
	},
	"kafka_sub":{
		"example_topic":{
			"addr":"127.0.0.1:12345",
			"username":"example",
			"password":"example",
			"group_name":"example_group",
			"start_offset":-1,
			"commit_interval":"0s"
		}
	}
}`
const textapp = `{

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
