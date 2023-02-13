package dao

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package dao

import (
	"net"
	"time"

	//"{{.}}/model"
	"{{.}}/config"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/web"
)

//var ExampleCGrpcApi example.ExampleCGrpcClient
//var ExampleCrpcApi example.ExampleCrpcClient
//var ExampleWebApi  example.ExampleWebClient

// NewApi create all dependent service's api we need in this program
func NewApi() error {
	var e error
	_ = e //avoid unuse

	cgrpcc := GetCGrpcClientConfig()
	_ = cgrpcc //avoid unuse

	//init cgrpc client below
	//examplecgrpc e = cgrpc.NewCGrpcClient(cgrpcc, model.Group, model.Name, "examplegroup", "examplename")
	//if e != nil {
	//         return e
	//}
	//ExampleCGrpcApi = example.NewExampleCGrpcClient(examplecgrpc)

	crpcc := GetCrpcClientConfig()
	_ = crpcc //avoid unuse

	//init crpc client below
	//examplecrpc, e = crpc.NewCrpcClient(crpcc, model.Group, model.Name, "examplegroup", "examplename")
	//if e != nil {
	// 	return e
	//}
	//ExampleCrpcApi = example.NewExampleCrpcClient(examplecrpc)

	webc := GetWebClientConfig()
	_ = webc //avoid unuse

	//init web client below
	//exampleweb, e = web.NewWebClient(webc, model.Group, model.Name, "examplegroup", "examplename", "http://examplehost:exampleport")
	//if e != nil {
	// 	return e
	//}
	//ExampleWebApi = example.NewExampleWebClient(exampleweb)

	return nil
}

func GetCGrpcClientConfig() *cgrpc.ClientConfig {
	gc := config.GetCGrpcClientConfig()
	return &cgrpc.ClientConfig{
		ConnectTimeout:   time.Duration(gc.ConnectTimeout),
		GlobalTimeout:    time.Duration(gc.GlobalTimeout),
		HeartProbe:       time.Duration(gc.HeartProbe),
		Discover:         cgrpcDNS,
		DiscoverInterval: time.Second * 10,
	}
}

func cgrpcDNS(group, name string) (map[string]*cgrpc.RegisterData, error) {
	result := make(map[string]*cgrpc.RegisterData)
	addrs, e := net.LookupHost(name + "-headless." + group)
	if e != nil {
		return nil, e
	}
	for i := range addrs {
		addrs[i] = addrs[i] + ":10000"
	}
	dserver := make(map[string]*struct{})
	dserver["dns"] = nil
	for _, addr := range addrs {
		result[addr] = &cgrpc.RegisterData{DServers: dserver}
	}
	return result,nil
}

func GetCrpcClientConfig() *crpc.ClientConfig {
	rc := config.GetCrpcClientConfig()
	return &crpc.ClientConfig{
		ConnectTimeout:   time.Duration(rc.ConnectTimeout),
		GlobalTimeout:    time.Duration(rc.GlobalTimeout),
		HeartProbe:       time.Duration(rc.HeartProbe),
		Discover:         crpcDNS,
		DiscoverInterval: time.Second * 10,
	}
}

func crpcDNS(group, name string) (map[string]*crpc.RegisterData, error) {
	result := make(map[string]*crpc.RegisterData)
	addrs, e := net.LookupHost(name + "-headless." + group)
	if e != nil {
		return nil, e
	}
	for i := range addrs {
		addrs[i] = addrs[i] + ":9000"
	}
	dserver := make(map[string]*struct{})
	dserver["dns"] = nil
	for _, addr := range addrs {
		result[addr] = &crpc.RegisterData{DServers: dserver}
	}
	return result, nil
}

func GetWebClientConfig() *web.ClientConfig {
	wc := config.GetWebClientConfig()
	return &web.ClientConfig{
		ConnectTimeout: time.Duration(wc.ConnectTimeout),
		GlobalTimeout:  time.Duration(wc.GlobalTimeout),
		IdleTimeout:    time.Duration(wc.IdleTimeout),
		HeartProbe:     time.Duration(wc.HeartProbe),
		MaxHeader:      2048,
	}
}`

const path = "./dao/"
const name = "dao.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("dao").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
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
func Execute(PackageName string) {
	if e := tml.Execute(file, PackageName); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
