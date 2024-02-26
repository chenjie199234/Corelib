package status

import (
	"os"
	"text/template"
)

const txt = `syntax="proto3";

//this is the app's name,all proto in this app must use this name as the proto package name
package {{.ProjectName}};
//this is the golang's package name,all proto in this project must use this name as the golang's package name
option go_package="{{.PackageName}}/api;api";
//https://github.com/chenjie199234/Corelib/blob/main/pbex/pbex.proto
import "pbex/pbex.proto";

//this is the proto file for status service
service status{
	//ping check server's health
	rpc ping(pingreq)returns(pingresp){
		option (pbex.method)="get";
		option (pbex.method)="crpc";
		option (pbex.method)="grpc";
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
	uint64 total_mem=3;
	uint64 cur_mem_usage=4;
	uint64 max_mem_usage=5;
	double cpu_num=6;
	double cur_cpu_usage=7;
	double avg_cpu_usage=8;
	double max_cpu_usage=9;
	string host=10;
	string ip=11;
}`

type data struct {
	PackageName string
	ProjectName string
}

func CreatePathAndFile(packagename, projectname string) {
	tmp := &data{
		PackageName: packagename,
		ProjectName: projectname,
	}
	if e := os.MkdirAll("./api/", 0755); e != nil {
		panic("mkdir ./api/ error: " + e.Error())
	}
	prototemplate, e := template.New("./api/" + projectname + "_status.proto").Parse(txt)
	if e != nil {
		panic("parse ./api/" + projectname + "_status.proto error: " + e.Error())
	}
	file, e := os.OpenFile("./api/"+projectname+"_status.proto", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./api/" + projectname + "_status.proto error: " + e.Error())
	}
	if e := prototemplate.Execute(file, tmp); e != nil {
		panic("write ./api/" + projectname + "_status.proto error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./api/" + projectname + "_status.proto error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./api/" + projectname + "_status.proto error: " + e.Error())
	}
}
