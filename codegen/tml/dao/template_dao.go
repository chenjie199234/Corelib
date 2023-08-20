package dao

import (
	"os"
	"text/template"
)

const txt = `package dao

import (
	"time"

	// "{{.}}/model"
	"{{.}}/config"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/web"
	// "github.com/chenjie199234/Corelib/discover"
)

//var ExampleCGrpcApi example.ExampleCGrpcClient
//var ExampleCrpcApi example.ExampleCrpcClient
//var ExampleWebApi  example.ExampleWebClient

// NewApi create all dependent service's api we need in this program
func NewApi() error {
	var e error
	_ = e //avoid unuse

	//init discover for example server
	//examplediscover := discover.NewDNSDiscover("exampleproject", "examplegroup", "examplename", "exampleproject-examplegroup.examplename-headless", time.Second*10, 9000, 10000, 8000)

	cgrpcc := GetCGrpcClientConfig()
	_ = cgrpcc //avoid unuse

	//init cgrpc client below
	//examplecgrpc, e = cgrpc.NewCGrpcClient(cgrpcc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", nil)
	//if e != nil {
	//         return e
	//}
	//ExampleCGrpcApi = example.NewExampleCGrpcClient(examplecgrpc)

	crpcc := GetCrpcClientConfig()
	_ = crpcc //avoid unuse

	//init crpc client below
	//examplecrpc, e = crpc.NewCrpcClient(crpcc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", nil)
	//if e != nil {
	// 	return e
	//}
	//ExampleCrpcApi = example.NewExampleCrpcClient(examplecrpc)

	webc := GetWebClientConfig()
	_ = webc //avoid unuse

	//init web client below
	//exampleweb, e = web.NewWebClient(webc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", nil)
	//if e != nil {
	// 	return e
	//}
	//ExampleWebApi = example.NewExampleWebClient(exampleweb)

	return nil
}

func UpdateAPI(ac *config.AppConfig) {

}

func GetCGrpcClientConfig() *cgrpc.ClientConfig {
	gc := config.GetCGrpcClientConfig()
	return &cgrpc.ClientConfig{
		ConnectTimeout:   time.Duration(gc.ConnectTimeout),
		GlobalTimeout:    time.Duration(gc.GlobalTimeout),
		HeartProbe:       time.Duration(gc.HeartProbe),
	}
}

func GetCrpcClientConfig() *crpc.ClientConfig {
	rc := config.GetCrpcClientConfig()
	return &crpc.ClientConfig{
		ConnectTimeout:   time.Duration(rc.ConnectTimeout),
		GlobalTimeout:    time.Duration(rc.GlobalTimeout),
		HeartProbe:       time.Duration(rc.HeartProbe),
	}
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

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./dao/", 0755); e != nil {
		panic("mkdir ./dao/ error: " + e.Error())
	}
	daotemplate, e := template.New("./dao/dao.go").Parse(txt)
	if e != nil {
		panic("parse ./dao/dao.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./dao/dao.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./dao/dao.go error: " + e.Error())
	}
	if e := daotemplate.Execute(file, packagename); e != nil {
		panic("write ./dao/dao.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./dao/dao.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./dao/dao.go error: " + e.Error())
	}
}
