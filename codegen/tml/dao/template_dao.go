package dao

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package dao

import (
//"encoding/json"
//"os"
//"time"

//"{{.}}/api"
//example "{{.}}/api/deps/example"
//"{{.}}/config"
)

//var ExampleRpcApi example.ExampleRpcClient
//var ExampleWebApi example.ExampleWebClient

//NewApi create all dependent service's api we need in this program
//example grpc client,http client
func NewApi() error {
	//var e error
	//rc := config.GetRpcConfig()
	//var verifydatas []string
	//if str, ok := os.LookupEnv("RPC_SERVER_VERIFY_DATA"); ok {
	//	if str == "<RPC_SERVER_VERIFY_DATA>" {
	//		str = ""
	//	}
	//	if str != "" {
	//		if e := json.Unmarshal([]byte(str), &verifydatas); e != nil {
	//			return e
	//		}
	//	}
	//}
	//rpcverifydata := ""
	//if len(verifydatas) != 0 {
	//	rpcverifydata = verifydatas[0]
	//}
	//if ExampleRpcApi, e = example.NewExampleRpcClient(time.Duration(rc.RpcTimeout), time.Duration(rc.RpcConnTimeout), time.Duration(rc.RpcHeartTimeout), time.Duration(rc.RpcHeartProbe), api.Group, api.Name, rpcverifydata, nil, nil); e != nil {
	//        return e
	//}
	//wc := config.GetWebConfig()
	//if ExampleWebApi, e = example.NewExampleWebClient(time.Duration(wc.WebTimeout), api.Group, api.Name, nil, nil); e != nil {
	//        return e
	//}
	return nil
}`

const path = "./dao/"
const name = "dao.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("dao").Parse(text)
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
