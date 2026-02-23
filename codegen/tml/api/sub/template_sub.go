package sub

import (
	"os"
	"text/template"
)

const txt = `edition = "2024";

//this is the app's name,all proto in this app must use this name as the proto package name
package {{.ProjectName}};
//this is the golang's package name,all proto in this project must use this name as the golang's package name
option go_package="{{.PackageName}}/api;api";
//https://github.com/chenjie199234/Corelib/blob/main/pbex/pbex.proto
import option "pbex/pbex.proto";

//this is the proto file for {{.Sname}} service
service {{.Sname}}{
	//for web server the response's 'Content-Type' always be 'application/json' when response code is not 200
	//for web server,you can set request's 'Accept' header to 'application/x-protobuf' to get the response data encoded by protobuf,and the response's 'Content-Type' will be setted to 'application/x-protobuf' otherwise it is setted to 'application/json'
	//rpc Example(ExampleReq)returns(RxampleResp){
	//	option (pbex.method)="get";
	//	option (pbex.method)="crpc";
	//	option (pbex.method)="grpc";//can be set to one of (get,delete,post,put,patch) or crpc or grpc
	//	option (pbex.web_midwares)="b";
	//	option (pbex.web_midwares)="c";
	//	option (pbex.web_midwares)="a";//this function on web protocol has 3 midwares,it's order is b,c,a
	//	option (pbex.crpc_midwares)="b";
	//	option (pbex.crpc_midwares)="c";
	//	option (pbex.crpc_midwares)="a";//this function on crpc protocol has 3 midwares,it's order is b,c,a
	//	option (pbex.cgrpc_midwares)="b";
	//	option (pbex.cgrpc_midwares)="c";
	//	option (pbex.cgrpc_midwares)="a";//this function on grpc protocol has 3 midwares,it's order is b,c,a
	//}

	//you can use the stream mode for both client and server on crpc and grpc
	//rpc ExampleStreamCrpcGrpc(stream ExampleReq)returns(stream ExampleResp){
	//	option (pbex.method)="crpc";
	//	option (pbex.method)="grpc";//can be set to one of (get,delete,post,put,patch) or crpc or grpc
	//	option (pbex.crpc_midwares)="b";
	//	option (pbex.crpc_midwares)="c";
	//	option (pbex.crpc_midwares)="a";//this function on crpc protocol has 3 midwares,it's order is b,c,a
	//	option (pbex.cgrpc_midwares)="b";
	//	option (pbex.cgrpc_midwares)="c";
	//	option (pbex.cgrpc_midwares)="a";//this function on grpc protocol has 3 midwares,it's order is b,c,a
	//}

	//1.for web server,you can only use the stream mode on server and the method must be 'get'(Server Sent Events,SSE mode)
	//2.unfortunate,javascript's 'EventSource' can't set header,so you need to use 'fetch' to simulate 'EventSource' if need to set header
	//3.response's 'Content-Type' always be 'application/json' when code is not 200 and always be 'text/event-stream' when code is 200.
	//4.only event:message(default) and event:error will be used,and event:error always be the last,after this,the connection will be closed
	//rpc ExampleStreamWeb(ExampleReq)returns(stream ExampleResp){
	//	option (pbex.method)="get";
	//	option (pbex.web_midwares)="b";
	//	option (pbex.web_midwares)="c";
	//	option (pbex.web_midwares)="a";//this function on web protocol has 3 midwares,it's order is b,c,a
	//}
}
//req can be set with pbex extentions
//message ExampleReq{
	//int64 example_for_extentions=1[(pbex.int_gt)=1,(pbex.int_lt)=100];
//}
//resp's pbex extentions will be ignore
//message ExampleResp{
	//int64 example_resp=1;
//}`

type data struct {
	PackageName string
	ProjectName string
	Sname       string
}

func CreatePathAndFile(packagename, projectname, sname string) {
	tmp := &data{
		PackageName: packagename,
		ProjectName: projectname,
		Sname:       string(sname[0]-32) + sname[1:],
	}
	if e := os.MkdirAll("./api/", 0755); e != nil {
		panic("mkdir ./api/ error: " + e.Error())
	}
	prototemplate, e := template.New("./api/" + projectname + "_" + sname + ".proto").Parse(txt)
	if e != nil {
		panic("parse ./api/" + projectname + "_" + sname + ".proto error: " + e.Error())
	}
	file, e := os.OpenFile("./api/"+projectname+"_"+sname+".proto", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./api/" + projectname + "_" + sname + ".proto error: " + e.Error())
	}
	if e := prototemplate.Execute(file, tmp); e != nil {
		panic("write ./api/" + projectname + "_" + sname + ".proto error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./api/" + projectname + "_" + sname + ".proto error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./api/" + projectname + "_" + sname + ".proto error: " + e.Error())
	}
}
