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

//this is the proto file for {{.Sname}} service
service {{.Sname}}{
	//comment option separate by '|'
	//method:     http method(only for http,rpc will ignore this),only support:get,post
	//timeout:    life control
	//mids:       midwares

	//rpc example(examplereq)returns(exampleresp);//method:get|timeout:200ms|mids:["examplemid1","examplemid2"]
}
//message examplereq{
	//comment option separate by '|',only request message support
	//header:     set to true,means this field comes from http's header(only for http,rpc will ignore this)
	//empty:      set to false,means this field can't be empty,only support bytes,string,repeated,map,message field
	//gt:         great then this value,only number kind field is useful
	//gte:        great or equal then this value,only number kind field is useful
	//lt:         less then this value,only number kind field is useful
	//lte:        less or equal then this value,only number kind field is useful
	//in:         value must in this collection(format:json string array),not support for map,message,repeated message field
	//notin:      value must not in this collection(format:json string array),not support for map,message,repeated message field

	//int64 example_for_comment_option=1;//header:true|gt:6|lt:666.6|gte:6.6|lte:666|in:["1","abc","3.14"]|notin:["1","abc","3.14"]
//}
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
