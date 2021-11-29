package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chenjie199234/Corelib/pbex"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	errorsPackage   = protogen.GoImportPath("errors")
	regexpPackage   = protogen.GoImportPath("regexp")
	contextPackage  = protogen.GoImportPath("context")
	protoPackage    = protogen.GoImportPath("google.golang.org/protobuf/proto")
	crpcPackage     = protogen.GoImportPath("github.com/chenjie199234/Corelib/crpc")
	logPackage      = protogen.GoImportPath("github.com/chenjie199234/Corelib/log")
	commonPackage   = protogen.GoImportPath("github.com/chenjie199234/Corelib/util/common")
	metadataPackage = protogen.GoImportPath("github.com/chenjie199234/Corelib/metadata")
	errorPackage    = protogen.GoImportPath("github.com/chenjie199234/Corelib/error")
)

func generateFile(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	filename := file.GeneratedFilenamePrefix + "_crpc.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)

	genFileComment(gen, file, g)

	g.P("package ", file.GoPackageName)
	g.P()

	for _, service := range file.Services {
		if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
			continue
		}
		genService(file, service, g)
	}
	return g
}
func genFileComment(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile) {
	//add version comments
	g.P("// Code generated by protoc-gen-go-crpc. DO NOT EDIT.")
	g.P("// version:")
	protocVersion := "(unknown)"
	if v := gen.Request.GetCompilerVersion(); v != nil {
		protocVersion = fmt.Sprintf("v%v.%v.%v", v.GetMajor(), v.GetMinor(), v.GetPatch())
		if s := v.GetSuffix(); s != "" {
			protocVersion += "-" + s
		}
	}
	g.P("// \tprotoc-gen-go-crpc ", version)
	g.P("// \tprotoc             ", protocVersion)
	g.P("// source: ", file.Desc.Path())
	g.P()
}

func genService(file *protogen.File, s *protogen.Service, g *protogen.GeneratedFile) {
	genPath(file, s, g)
	genClient(file, s, g)
	genServer(file, s, g)
}

func genPath(file *protogen.File, service *protogen.Service, g *protogen.GeneratedFile) {
	for _, method := range service.Methods {
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			continue
		}
		pathname := "_CrpcPath" + service.GoName + method.GoName
		pathurl := "/" + *file.Proto.Package + "." + string(service.Desc.Name()) + "/" + string(method.Desc.Name())
		g.P("var ", pathname, "=", strconv.Quote(pathurl))
	}
	g.P()
}
func genServer(file *protogen.File, service *protogen.Service, g *protogen.GeneratedFile) {
	// Server interface.
	serverName := service.GoName + "CrpcServer"

	g.P("type ", serverName, " interface {")
	for _, method := range service.Methods {
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			continue
		}
		g.P(method.Comments.Leading,
			method.GoName, "(", g.QualifiedGoIdent(contextPackage.Ident("Context")), ",*", g.QualifiedGoIdent(method.Input.GoIdent), ")(*", g.QualifiedGoIdent(method.Output.GoIdent), ",error)",
			method.Comments.Trailing)
	}
	g.P("}")
	g.P()
	// Server handler
	for _, method := range service.Methods {
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			continue
		}
		fname := "func _" + service.GoName + "_" + method.GoName + "_" + "CrpcHandler"
		p1 := "handler func (" + g.QualifiedGoIdent(contextPackage.Ident("Context")) + ",*" + g.QualifiedGoIdent(method.Input.GoIdent) + ")(*" + g.QualifiedGoIdent(method.Output.GoIdent) + ",error)"
		freturn := g.QualifiedGoIdent(crpcPackage.Ident("OutsideHandler"))
		g.P(fname, "(", p1, ")", freturn, "{")
		g.P("return func(stdctx ", g.QualifiedGoIdent(contextPackage.Ident("Context")), "){")
		g.P("ctx:=stdctx.(*", g.QualifiedGoIdent(crpcPackage.Ident("Context")), ")")
		g.P("req:=new(", g.QualifiedGoIdent(method.Input.GoIdent), ")")
		g.P("if e:=", g.QualifiedGoIdent(protoPackage.Ident("Unmarshal")), "(ctx.GetBody(),req);e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
		g.P("return")
		g.P("}")

		pathurl := "/" + *file.Proto.Package + "." + string(service.Desc.Name()) + "/" + string(method.Desc.Name())
		if pbex.NeedCheck(method.Input) {
			g.P("if errstr := req.Validate(); errstr != \"\" {")
			g.P(g.QualifiedGoIdent(logPackage.Ident("Error")), "(ctx,\"[", pathurl, "]\",errstr)")
			g.P("ctx.Abort(", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
			g.P("return")
			g.P("}")
		}

		g.P("resp,e:=handler(ctx,req)")
		g.P("if e!=nil{")
		g.P("ctx.Abort(e)")
		g.P("return")
		g.P("}")
		g.P("if resp == nil{")
		g.P("resp = new(", g.QualifiedGoIdent(method.Output.GoIdent), ")")
		g.P("}")
		g.P("respd,_:=", g.QualifiedGoIdent(protoPackage.Ident("Marshal")), "(resp)")
		g.P("ctx.Write(respd)")
		g.P("}")
		g.P("}")
	}

	//Server Register
	g.P("func Register", serverName, "(engine *", g.QualifiedGoIdent(crpcPackage.Ident("CrpcServer")), ",svc ", serverName, ",allmids map[string]", g.QualifiedGoIdent(crpcPackage.Ident("OutsideHandler")), "){")
	g.P("//avoid lint")
	g.P("_=allmids")
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		var mids []string
		if proto.HasExtension(mop, pbex.E_CrpcMidwares) {
			mids = proto.GetExtension(mop, pbex.E_CrpcMidwares).([]string)
		}
		fname := "_" + service.GoName + "_" + method.GoName + "_" + "CrpcHandler(svc." + method.GoName + ")"
		pathname := "_CrpcPath" + service.GoName + method.GoName
		if len(mids) > 0 {
			g.P("{")
			str := ""
			for _, mid := range mids {
				str += ","
				str += strconv.Quote(mid)
			}
			str = str[1:]
			g.P("requiredMids:=[]string{", str, "}")
			g.P("mids:=make([]", g.QualifiedGoIdent(crpcPackage.Ident("OutsideHandler")), ",0,", len(mids)+1, ")")
			g.P("for _,v:=range requiredMids{")
			g.P("if mid,ok:=allmids[v];ok{")
			g.P("mids = append(mids,mid)")
			g.P("}else{")
			g.P("panic(\"missing midware:\"+v)")
			g.P("}")
			g.P("}")
			g.P("mids = append(mids,", fname, ")")
			g.P("engine.RegisterHandler(", pathname, ",mids...)")
			g.P("}")
		} else {
			g.P("engine.RegisterHandler(", pathname, ",", fname, ")")
		}
	}
	g.P("}")
}
func genClient(file *protogen.File, service *protogen.Service, g *protogen.GeneratedFile) {
	// Client interface.
	clientName := service.GoName + "CrpcClient"
	lowclientName := strings.ToLower(clientName[:1]) + clientName[1:]

	g.P("type ", clientName, " interface {")
	for _, method := range service.Methods {
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			continue
		}
		g.P(method.Comments.Leading,
			method.GoName, "(", g.QualifiedGoIdent(contextPackage.Ident("Context")), ",*", g.QualifiedGoIdent(method.Input.GoIdent), ")(*", g.QualifiedGoIdent(method.Output.GoIdent), ",error)",
			method.Comments.Trailing)
	}
	g.P("}")
	g.P()
	g.P("type ", lowclientName, " struct{")
	g.P("cc *", g.QualifiedGoIdent(crpcPackage.Ident("CrpcClient")))
	g.P("}")
	g.P("func New", clientName, "(c *", g.QualifiedGoIdent(crpcPackage.Ident("CrpcClient")), ")(", clientName, "){")
	g.P("return &", lowclientName, "{cc:c}")
	g.P("}")
	g.P()
	// Client handler
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		pathname := "_CrpcPath" + service.GoName + method.GoName
		p1 := "ctx " + g.QualifiedGoIdent(contextPackage.Ident("Context"))
		p2 := "req *" + g.QualifiedGoIdent(method.Input.GoIdent)
		freturn := "(*" + g.QualifiedGoIdent(method.Output.GoIdent) + ",error)"
		g.P("func (c *", lowclientName, ")", method.GoName, "(", p1, ",", p2, ")", freturn, "{")
		g.P("if req == nil {")
		g.P("return nil,", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")))
		g.P("}")

		g.P("reqd,_:=", g.QualifiedGoIdent(protoPackage.Ident("Marshal")), "(req)")
		g.P("respd,e:=c.cc.Call(ctx,", pathname, ",reqd,", metadataPackage.Ident("GetMetadata"), "(ctx))")
		g.P("if e != nil {")
		g.P("return nil,e")
		g.P("}")
		g.P("resp := new(", g.QualifiedGoIdent(method.Output.GoIdent), ")")
		g.P("if len(respd)==0{")
		g.P("return resp,nil")
		g.P("}")
		g.P("if e:=", g.QualifiedGoIdent(protoPackage.Ident("Unmarshal")), "(respd,resp);e!=nil{")
		g.P("return nil,", g.QualifiedGoIdent(errorPackage.Ident("ErrResp")))
		g.P("}")
		g.P("return resp, nil")
		g.P("}")
	}
}
