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

	//"{{.}}/api"
	//example "{{.}}/api/deps/example"
	"{{.}}/config"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/rpc"
	"github.com/chenjie199234/Corelib/web"
)

//var ExampleRpcApi example.ExampleRpcClient
//var ExampleWebApi example.ExampleWebClient

//NewApi create all dependent service's api we need in this program
//example grpc client,http client
func NewApi() error {
	var e error
	_ = e //avoid unuse

	rpcc := getRpcClientConfig()
	_ = rpcc //avoid unuse

	//init rpc client below
	//if ExampleRpcApi, e = example.NewExampleRpcClient(rpcc, api.Group, api.Name); e != nil {
	//        return e
	//}

	webc := getWebClientConfig()
	_ = webc //avoid unuse

	//init web client below
	//if ExampleWebApi, e = example.NewExampleWebClient(webc, api.Group, api.Name); e != nil {
	//        return e
	//}
	return nil
}
func getRpcClientConfig() *rpc.ClientConfig {
	rc := config.GetRpcClientConfig()
	rpcverifydata := ""
	if len(config.EC.ServerVerifyDatas) != 0 {
		rpcverifydata = config.EC.ServerVerifyDatas[0]
	}
	return &rpc.ClientConfig{
		ConnTimeout:            time.Duration(rc.ConnTimeout),
		GlobalTimeout:          time.Duration(rc.GlobalTimeout),
		HeartTimeout:           time.Duration(rc.HeartTimeout),
		HeartPorbe:             time.Duration(rc.HeartProbe),
		GroupNum:               1,
		SocketRBuf:             2048,
		SocketWBuf:             2048,
		MaxMsgLen:              65535,
		MaxBufferedWriteMsgNum: 1024,
		VerifyData:             rpcverifydata,
		DiscoverFunction:       rpcDNS(),
	}
}

func rpcDNS() func(string, string, <-chan struct{}) (map[string]*rpc.RegisterData, error) {
	tker := time.NewTicker(time.Second * 10)
	return func(group, name string, manually <-chan struct{}) (map[string]*rpc.RegisterData, error) {
		select {
		case <-tker.C:
		case <-manually:
		}
		result := make(map[string]*rpc.RegisterData)
		addrs, e := net.LookupHost(name + "-service-headless" + "." + group)
		if e != nil {
			log.Error(nil,"[rpc.dns] get:", name+"-service-headless", "addrs error:", e)
			return nil, e
		}
		for i := range addrs {
			addrs[i] = addrs[i] + ":9000"
		}
		dserver := make(map[string]struct{})
		dserver["dns"] = struct{}{}
		for _, addr := range addrs {
			result[addr] = &rpc.RegisterData{DServers: dserver}
		}
		for len(tker.C) > 0 {
			<-tker.C
		}
		return result, nil
	}
}

func getWebClientConfig() *web.ClientConfig {
	wc := config.GetWebClientConfig()
	return &web.ClientConfig{
		GlobalTimeout:    time.Duration(wc.GlobalTimeout),
		IdleTimeout:      time.Duration(wc.IdleTimeout),
		HeartProbe:       time.Duration(wc.HeartProbe),
		MaxHeader:        1024,
		SocketRBuf:       2048,
		SocketWBuf:       2048,
		SkipVerifyTLS:    wc.SkipVerifyTls,
		DiscoverFunction: webDNS(),
	}
}

func webDNS() func(string, string, <-chan struct{}) (map[string]*web.RegisterData, error) {
	tker := time.NewTicker(time.Second * 10)
	return func(group, name string, manually <-chan struct{}) (map[string]*web.RegisterData, error){
		select {
		case <-tker.C:
		case <-manually:
		}
		result := make(map[string]*web.RegisterData)
		addrs, e := net.LookupHost(name + "-service-headless" + "." + group)
		if e != nil {
			log.Error(nil,"[web.dns] get:", name+"-service-headless", "addrs error:", e)
			return nil, e
		}
		for i := range addrs {
			addrs[i] = addrs[i] + ":8000"
		}
		dserver := make(map[string]struct{})
		dserver["dns"] = struct{}{}
		for _, addr := range addrs {
			result[addr] = &web.RegisterData{DServers: dserver}
		}
		for len(tker.C) > 0 {
			<-tker.C
		}
		return result, nil
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
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
