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

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/grpc"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/web"
)

//var ExampleGrpcApi example.ExampleGrpcClient
//var ExampleCrpcApi example.ExampleCrpcClient
//var ExampleWebApi  example.ExampleWebClient

//NewApi create all dependent service's api we need in this program
func NewApi() error {
	var e error
	_ = e //avoid unuse

	grpcc := getGrpcClientConfig()
	_ = grpcc //avoid unuse

	//init grpc client below
	//examplegrpc e = grpc.NewGrpcClient(grpcc, api.Group, api.Name, "examplegroup", "examplename")
	//if e != nil {
	//         return e
	//}
	//ExampleGrpcApi = example.NewExampleGrpcClient(examplegrpc)

	crpcc := getCrpcClientConfig()
	_ = crpcc //avoid unuse

	//init crpc client below
	//examplecrpc, e = crpc.NewCrpcClient(crpcc, api.Group, api.Name, "examplegroup", "examplename")
	//if e != nil {
	// 	return e
	//}
	//ExampleCrpcApi = example.NewExampleCrpcClient(examplecrpc)

	webc := getWebClientConfig()
	_ = webc //avoid unuse

	//init web client below
	//exampleweb, e = web.NewWebClient(webc, api.Group, api.Name, "examplegroup", "examplename")
	//if e != nil {
	// 	return e
	//}
	//ExampleWebApi = example.NewExampleWebClient(exampleweb)

	return nil
}

func getGrpcClientConfig() *grpc.ClientConfig {
	gc := config.GetGrpcClientConfig()
	return &grpc.ClientConfig{
		ConnTimeout:   time.Duration(gc.ConnTimeout),
		GlobalTimeout: time.Duration(gc.GlobalTimeout),
		HeartPorbe:    time.Duration(gc.HeartProbe),
		SocketRBuf:    2048,
		SocketWBuf:    2048,
		MaxMsgLen:     65535,
		Discover:      grpcDNS,
	}
}

func grpcDNS(group, name string, manually <-chan *struct{}, client *grpc.GrpcClient) {
	tker := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-tker.C:
		case <-manually:
			tker.Reset(time.Second * 10)
		}
		result := make(map[string]*grpc.RegisterData)
		addrs, e := net.LookupHost(name + "-service-headless" + "." + group)
		if e != nil {
			log.Error(nil, "[grpc.dns] get:", name+"-service-headless", "addrs error:", e)
			continue
		}
		for i := range addrs {
			addrs[i] = addrs[i] + ":7000"
		}
		dserver := make(map[string]struct{})
		dserver["dns"] = struct{}{}
		for _, addr := range addrs {
			result[addr] = &grpc.RegisterData{DServers: dserver}
		}
		for len(tker.C) > 0 {
			<-tker.C
		}
		client.UpdateDiscovery(result)
	}
}

func getCrpcClientConfig() *crpc.ClientConfig {
	rc := config.GetCrpcClientConfig()
	return &crpc.ClientConfig{
		ConnTimeout:   time.Duration(rc.ConnTimeout),
		GlobalTimeout: time.Duration(rc.GlobalTimeout),
		HeartPorbe:    time.Duration(rc.HeartProbe),
		SocketRBuf:    2048,
		SocketWBuf:    2048,
		MaxMsgLen:     65535,
		Discover:      crpcDNS,
	}
}

func crpcDNS(group, name string, manually <-chan *struct{}, client *crpc.CrpcClient) {
	tker := time.NewTicker(time.Second * 10)
	for{
		select {
		case <-tker.C:
		case <-manually:
			tker.Reset(time.Second * 10)
		}
		result := make(map[string]*crpc.RegisterData)
		addrs, e := net.LookupHost(name + "-service-headless" + "." + group)
		if e != nil {
			log.Error(nil,"[crpc.dns] get:", name+"-service-headless", "addrs error:", e)
			continue
		}
		for i := range addrs {
			addrs[i] = addrs[i] + ":9000"
		}
		dserver := make(map[string]struct{})
		dserver["dns"] = struct{}{}
		for _, addr := range addrs {
			result[addr] = &crpc.RegisterData{DServers: dserver}
		}
		for len(tker.C) > 0 {
			<-tker.C
		}
		client.UpdateDiscovery(result)
	}
}

func getWebClientConfig() *web.ClientConfig {
	wc := config.GetWebClientConfig()
	return &web.ClientConfig{
		ConnTimeout:   time.Duration(wc.ConnTimeout),
		GlobalTimeout: time.Duration(wc.GlobalTimeout),
		IdleTimeout:   time.Duration(wc.IdleTimeout),
		HeartProbe:    time.Duration(wc.HeartProbe),
		MaxHeader:     1024,
		SocketRBuf:    2048,
		SocketWBuf:    2048,
		Discover:      webDNS,
	}
}

func webDNS(group, name string, manually <-chan *struct{}, client *web.WebClient) {
	tker := time.NewTicker(time.Second * 10)
	for{
		select {
		case <-tker.C:
		case <-manually:
			tker.Reset(time.Second * 10)
		}
		result := make(map[string]*web.RegisterData)
		addrs, e := net.LookupHost(name + "-service-headless" + "." + group)
		if e != nil {
			log.Error(nil,"[web.dns] get:", name+"-service-headless", "addrs error:", e)
			continue
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
		client.UpdateDiscovery(result)
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
