package configfile

import (
	"os"
	"text/template"
)

const source = `{
	"cgrpc_server":{
		"connect_timeout":"500ms",
		"idle_timeout":"0",
		"global_timeout":"500ms",
		"heart_probe":"10s",
		"certs":{
		}
	},
	"cgrpc_client":{
		"connect_timeout":"500ms",
		"idle_timeout":"0",
		"global_timeout":"0",
		"heart_probe":"10s"
	},
	"crpc_server":{
		"connect_timeout":"500ms",
		"idle_timeout":"0",
		"global_timeout":"500ms",
		"heart_probe":"10s",
		"certs":{
		}
	},
	"crpc_client":{
		"connect_timeout":"500ms",
		"idle_timeout":"0",
		"global_timeout":"0",
		"heart_probe":"10s"
	},
	"web_server":{
		"wait_close_mode":0,
		"wait_close_time":"1s",
		"connect_timeout":"500ms",
		"global_timeout":"500ms",
		"idle_timeout":"5s",
		"max_request_header":4096,
		"cors_allowed_origins":["*"],
		"cors_allowed_headers":["*"],
		"cors_expose_headers":["*"],
		"cors_allow_credentials":false,
		"cors_max_age":"30m",
		"src_root_path":"",
		"certs":{
		}
	},
	"web_client":{
		"connect_timeout":"500ms",
		"global_timeout":"0",
		"idle_timeout":"5s",
		"max_response_header":4096
	},
	"mongo":{
		"example_mongo":{
			"tls":false,
			"specific_ca_paths":["./example.pem"],
			"srv_name":"",
			"addrs":["127.0.0.1:27017"],
			"user_name":"",
			"password":"",
			"auth_db":"",
			"replica_set":"",
			"max_open":256,
			"max_conn_idletime":"5m",
			"io_timeout":"500ms",
			"dial_timeout":"250ms"
		}
	},
	"mysql":{
		"example_mysql":{
			"tls":false,
			"specific_ca_paths":["./example.pem"],
			"addr":"127.0.0.1:3306",
			"user_name":"",
			"password":"",
			"max_open":256,
			"max_conn_idletime":"5m",
			"io_timeout":"500ms",
			"dial_timeout":"250ms",
			"charset":"",
			"collation":"",
			"parse_time":true
		}
	},
	"redis":{
		"example_redis":{
			"tls":false,
			"specific_ca_paths":["./example.pem"],
			"addrs":["127.0.0.1:6379"],
			"user_name":"",
			"password":"",
			"max_open":256,
			"max_conn_idletime":"5m",
			"io_timeout":"500ms",
			"dial_timeout":"250ms"
		}
	}
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
