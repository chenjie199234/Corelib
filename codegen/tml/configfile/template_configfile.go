package configfile

import (
	"fmt"
	"os"
	"text/template"
)

const textsource = `{
	"rpc":{
		"rpc_port":9000,
		"rpc_timeout":"200ms"
	},
	"http":{
		"http_port":8000,
		"http_timeout":"200ms",
		"http_certfile":"",
		"http_keyfile":"",
		"http_cors":{
			"cors_origin":["*"],
			"cors_method":["GET","POST","OPTIONS"],
			"cors_header":["*"],
			"cors_expose":[]
		}
	},
	"db":{
		"example_db":{
			"username":"root",
			"passwd":"root",
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
			"commit_interval":0
		}
	}
}`
const textapp = `{

}`
const textdiscovery = `{
	"http":{
		"example_service":[]
	},
	"grpc":{
		"example_service":[]
	}
}`

const path = "./"
const sourcename = "SourceConfig.json"
const appname = "AppConfig.json"
const discoveryname = "DiscoveryConfig.json"

var tmlsource *template.Template
var tmlapp *template.Template
var tmldiscovery *template.Template

var filesource *os.File
var fileapp *os.File
var filediscovery *os.File

func init() {
	var e error
	tmlsource, e = template.New("source").Parse(textsource)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+sourcename, e))
	}
	tmlapp, e = template.New("app").Parse(textapp)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+appname, e))
	}
	tmldiscovery, e = template.New("discovery").Parse(textdiscovery)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+discoveryname, e))
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
	filediscovery, e = os.OpenFile(path+discoveryname, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+discoveryname, e))
	}
}
func Execute(projectname string) {
	if e := tmlsource.Execute(filesource, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+sourcename, e))
	}
	if e := tmlapp.Execute(fileapp, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+appname, e))
	}
	if e := tmldiscovery.Execute(filediscovery, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+discoveryname, e))
	}
}
