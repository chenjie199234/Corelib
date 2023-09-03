package configfile

import (
	"os"
	"text/template"
)

const source = `{
	"cgrpc_server":{
		"connect_timeout":"200ms",
		"global_timeout":"500ms",
		"heart_probe":"10s",
		"certs":{
		}
	},
	"cgrpc_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"heart_probe":"10s"
	},
	"crpc_server":{
		"connect_timeout":"200ms",
		"global_timeout":"500ms",
		"heart_probe":"3s",
		"certs":{
		}
	},
	"crpc_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"heart_probe":"3s"
	},
	"web_server":{
		"close_mode":0,
		"connect_timeout":"200ms",
		"global_timeout":"500ms",
		"idle_timeout":"10s",
		"heart_probe":"3s",
		"src_root":"",
		"certs":{
		},
		"web_cors":{
			"cors_origin":["*"],
			"cors_header":["*"],
			"cors_expose":[]
		}
	},
	"web_client":{
		"connect_timeout":"200ms",
		"global_timeout":"0",
		"idle_timeout":"10s",
		"heart_probe":"3s"
	},
	"mongo":{
		"example_mongo":{
			"url":"[mongodb/mongodb+srv]://[username:password@]host1,...,hostN[/dbname][?param1=value1&...&paramN=valueN]",
			"max_open":256,
			"max_idletime":"10m",
			"io_timeout":"500ms",
			"conn_timeout":"250ms"
		}
	},
	"sql":{
		"example_sql":{
			"url":"[username:password@][protocol(address)]/[dbname][?param1=value1&...&paramN=valueN]",
			"max_open":256,
			"max_idle":100,
			"max_idletime":"10m",
			"io_timeout":"500ms",
			"conn_timeout":"250ms"
		}
	},
	"redis":{
		"example_redis":{
			"url":"[redis/rediss]://[[username:]password@]host[/dbindex]",
			"max_open":256,
			"max_idle":100,
			"max_idletime":"10m",
			"io_timeout":"500ms",
			"conn_timeout":"250ms"
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
			"conn_timeout":"250ms"
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
			"conn_timeout":"250ms",
			"start_offset":-2,
			"commit_interval":"0s"
		}
	]
}`
const app = `{
	"handler_timeout":{
		"/{{.}}.status/ping":{
			"GET":"200ms",
			"CRPC":"200ms",
			"GRPC":"200ms"
		}
	},
	"web_path_rewrite":{
		"GET":{
			"/origin/url":"/{{.}}.exampleservice/examplemethod"
		}
	},
	"handler_rate":{
		"/{{.}}.exampleservice/examplemethod":[{
			"methods":["GET","GRPC","CRPC"],
			"max_rate":10,
			"period":1,
			"rate_type":"path"
		},{
			"methods":["GET","GRPC","CRPC"],
			"max_rate":10,
			"period":1,
			"rate_type":"token"
		}]
	},
	"accesses":{
		"/{{.}}.exampleservice/examplemethod":[{
			"method":["GET","GRPC","CRPC"],
			"accesses":{
				"accessid":"accesskey"
			}
		}]
	},
	"token_secret":"test",
	"session_token_expire":"24h",
	"service":{

	}
}`

func CreatePathAndFile(projectname string) {
	//./SourceConfig.json
	sourcefile, e := os.OpenFile("./SourceConfig.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./SourceConfig.json error: " + e.Error())
	}
	if _, e := sourcefile.WriteString(source); e != nil {
		panic("write ./SourceConfig.json error: " + e.Error())
	}
	if e := sourcefile.Sync(); e != nil {
		panic("sync ./SourceConfig.json error: " + e.Error())
	}
	if e := sourcefile.Close(); e != nil {
		panic("close ./SourceConfig.json error: " + e.Error())
	}
	//./AppConfig.json
	apptemplate, e := template.New("./AppConfig.json").Parse(app)
	if e != nil {
		panic("parse ./AppConfig.json template error: " + e.Error())
	}
	appfile, e := os.OpenFile("./AppConfig.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./AppConfig.json error: " + e.Error())
	}
	if e := apptemplate.Execute(appfile, projectname); e != nil {
		panic("write ./AppConfig.json error: " + e.Error())
	}
	if e := appfile.Sync(); e != nil {
		panic("sync ./AppConfig.json error: " + e.Error())
	}
	if e := appfile.Close(); e != nil {
		panic("close ./AppConfig.json error: " + e.Error())
	}
}
