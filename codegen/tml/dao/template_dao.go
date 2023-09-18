package dao

import (
	"os"
	"text/template"
)

const txt = `package dao

import (
	"{{.}}/config"
	// "{{.}}/model"
	// "github.com/chenjie199234/Corelib/discover"
	// "github.com/chenjie199234/Corelib/cgrpc"
	// "github.com/chenjie199234/Corelib/crpc"
	// "github.com/chenjie199234/Corelib/web"
)

//var ExampleCGrpcApi example.ExampleCGrpcClient
//var ExampleCrpcApi example.ExampleCrpcClient
//var ExampleWebApi  example.ExampleWebClient

// NewApi create all dependent service's api we need in this program
func NewApi() error {
	//init discover for example server
	//examplediscover, e := discover.NewDNSDiscover("exampleproject", "examplegroup", "examplename", "exampleproject-examplegroup.examplename-headless", time.Second*10, 9000, 10000, 8000)
	//if e != nil {
	//	return e
	//}

	//if need tls,you need to specific the config
	//var exampletlsc *tls.Config
	// if needtls {
	// 	exampletlsc = &tls.Config{}
	// 	...
	// }

	cgrpcc := config.GetCGrpcClientConfig().ClientConfig
	_ = cgrpcc //avoid unuse

	//init cgrpc client below
	//examplecgrpc, e = cgrpc.NewCGrpcClient(cgrpcc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	//         return e
	//}
	//ExampleCGrpcApi = example.NewExampleCGrpcClient(examplecgrpc)

	crpcc := config.GetCrpcClientConfig().ClientConfig
	_ = crpcc //avoid unuse

	//init crpc client below
	//examplecrpc, e = crpc.NewCrpcClient(crpcc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	// 	return e
	//}
	//ExampleCrpcApi = example.NewExampleCrpcClient(examplecrpc)

	webc := config.GetWebClientConfig().ClientConfig
	_ = webc //avoid unuse

	//init web client below
	//exampleweb, e = web.NewWebClient(webc, examplediscover, model.Project, model.Group, model.Name, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	// 	return e
	//}
	//ExampleWebApi = example.NewExampleWebClient(exampleweb)

	return nil
}

func UpdateAPI(ac *config.AppConfig) {

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
