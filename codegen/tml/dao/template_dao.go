package dao

import (
	"os"
	"text/template"
)

const txt = `package dao

import (
	"{{.}}/config"
	// "{{.}}/model"

	// admindiscover "github.com/chenjie199234/admin/sdk/discover"
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
	//init dns discover for example server
	//exampleDnsDiscover, e := discover.NewDNSDiscover("exampleproject", "examplegroup", "examplename", "dnshost", time.Second*10, 9000, 10000, 8000)
	//if e != nil {
	//	return e
	//}
	//
	//init static discover for example server
	//exampleStaticDiscover, e := discover.NewStaticDiscover("exampleproject", "examplegroup", "examplename", []string{"addr1","addr2"}, 9000, 10000, 8000)
	//if e != nil {
	//	return e
	//}
	//
	//init kubernetes discover for example server
	//exampleKubeDiscover, e := discover.NewKubernetesDiscover("exampleproject", "examplegroup", "examplename", "namespace", "fieldselector", "labelselector", 9000, 10000, 8000)
	//if e != nil {
	//	return e
	//}
	//
	//init admin discover for example server
	//if admin service needs tls,you need to specific the config
	//var admintlsc *tlsc.Config{}
	//if adminNeedTLS {
	//	admintlsc = &tlsc.Config{}
	//	...
	//}
	//exampleAdminDiscover, e := admindiscover.NewAdminDiscover("exampleproject", "examplegroup", "examplename", admintlsc)
	//if e != nil {
	//	return e
	//}

	//if example service needs tls,you need to specific the config
	//var exampletlsc *tls.Config
	// if exampleNeedTLS {
	// 	exampletlsc = &tls.Config{}
	// 	...
	// }

	cgrpcc := config.GetCGrpcClientConfig().ClientConfig
	_ = cgrpcc //avoid unuse

	//init cgrpc client below
	//examplecgrpc, e := cgrpc.NewCGrpcClient(cgrpcc, examplediscover, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	//         return e
	//}
	//ExampleCGrpcApi = example.NewExampleCGrpcClient(examplecgrpc)

	crpcc := config.GetCrpcClientConfig().ClientConfig
	_ = crpcc //avoid unuse

	//init crpc client below
	//examplecrpc, e := crpc.NewCrpcClient(crpcc, examplediscover, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	// 	return e
	//}
	//ExampleCrpcApi = example.NewExampleCrpcClient(examplecrpc)

	webc := config.GetWebClientConfig().ClientConfig
	_ = webc //avoid unuse

	//init web client below
	//exampleweb, e := web.NewWebClient(webc, examplediscover, "exampleproject", "examplegroup", "examplename", exampletlsc)
	//if e != nil {
	// 	return e
	//}
	//ExampleWebApi = example.NewExampleWebClient(exampleweb)

	return nil
}

func UpdateAppConfig(ac *config.AppConfig) {

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
