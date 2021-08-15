package sub

import (
	"fmt"
	"os"
	"text/template"
)

const text = `syntax="proto3";

//this is the proto package name,all proto in this project must use this name as the proto package name
package {{.Pname}};
//this is the golang's package name,all proto in this project must use this name as the golang's package name
option go_package="{{.Pname}}/api;api";
import "pbex/pbex.proto";

//this is the proto file for {{.Sname}} service
service {{.Sname}}{
	//rpc example(examplereq)returns(exampleresp){
	//	option (pbex.method)="get";//can be set to get,delete,post,put,patch
	//	option (pbex.timeout)="200ms";//1ns 1us 1ms 1s 1m 1h,without this means no function timeout
	//	option (pbex.midwares)="auth";
	//	option (pbex.midwares)="c";
	//	option (pbex.midwares)="a";//this function has 3 midwares,it's order is auth,c,a
	//}
}
//req can be set with pbex extentions
//message examplereq{
	//int64 example_for_extentions=1[(pbex.int_gt)=1,(pbex.int_lt)=100];
//}
//resp's pbex extentions will be ignore
//message exampleresp{
	//int64 example_resp=1;
//}`

const path = "./api/"

var tml *template.Template
var file *os.File

type data struct {
	Pname string
	Sname string
}

func init() {
	var e error
	tml, e = template.New("api").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile(sname string) {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+sname+".proto", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+".proto", e))
	}
}
func Execute(pname, sname string) {
	if e := tml.Execute(file, &data{Pname: pname, Sname: sname}); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+sname+".proto", e))
	}
}
