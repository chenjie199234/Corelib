package status

import (
	"fmt"
	"os"
	"text/template"
)

const text = `syntax="proto3";

//this is the proto package name,all proto in this project must use this name as the proto package name
package {{.}};
//this is the golang's package name,all proto in this project must use this name as the golang's package name
option go_package="{{.}}/api;api";
//https://github.com/chenjie199234/Corelib/blob/main/pbex/pbex.proto
import "pbex/pbex.proto";

//this is the proto file for status service
service status{
	//ping check server's health
	rpc ping(pingreq)returns(pingresp){
		option (pbex.method)="get";
		option (pbex.web_midwares)="accesskey";
		option (pbex.web_midwares)="rate";
		option (pbex.crpc_midwares)="accesskey";
		option (pbex.crpc_midwares)="rate";
		option (pbex.cgrpc_midwares)="accesskey";
		option (pbex.cgrpc_midwares)="rate";
	}
}
//req can be set with pbex extentions
message pingreq{
	int64 timestamp=1[(pbex.int_gt)=0];
}
//resp's pbex extentions will be ignore
message pingresp{
	int64 client_timestamp=1;
	int64 server_timestamp=2;
}`

const path = "./api/"
const name = "status_%s.proto"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("api").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile(projectname string) {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+fmt.Sprintf(name, projectname), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+fmt.Sprintf(name, projectname), e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+fmt.Sprintf(name, projectname), e))
	}
}
