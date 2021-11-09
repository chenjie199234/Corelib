package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/pbex"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	errorsPackage    = protogen.GoImportPath("errors")
	stringsPackage   = protogen.GoImportPath("strings")
	httpPackage      = protogen.GoImportPath("net/http")
	regexpPackage    = protogen.GoImportPath("regexp")
	ioPackage        = protogen.GoImportPath("io")
	contextPackage   = protogen.GoImportPath("context")
	stdjsonPackage   = protogen.GoImportPath("encoding/json")
	protoPackage     = protogen.GoImportPath("google.golang.org/protobuf/proto")
	protojsonPackage = protogen.GoImportPath("google.golang.org/protobuf/encoding/protojson")
	webPackage       = protogen.GoImportPath("github.com/chenjie199234/Corelib/web")
	commonPackage    = protogen.GoImportPath("github.com/chenjie199234/Corelib/util/common")
	metadataPackage  = protogen.GoImportPath("github.com/chenjie199234/Corelib/metadata")
	bufpoolPackage   = protogen.GoImportPath("github.com/chenjie199234/Corelib/bufpool")
	errorPackage     = protogen.GoImportPath("github.com/chenjie199234/Corelib/error")
	logPackage       = protogen.GoImportPath("github.com/chenjie199234/Corelib/log")
)

// generateFile generates a _web.pb.go file containing web service definitions.
func generateFile(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	filename := file.GeneratedFilenamePrefix + "_web.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)

	genFileComment(gen, file, g)

	g.P("package ", file.GoPackageName)
	g.P()

	for _, service := range file.Services {
		if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
			continue
		}
		genService(file, g, service)
	}
	return g
}
func genFileComment(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile) {
	//add version comments
	g.P("// Code generated by protoc-gen-go-web. DO NOT EDIT.")
	g.P("// version:")
	protocVersion := "(unknown)"
	if v := gen.Request.GetCompilerVersion(); v != nil {
		protocVersion = fmt.Sprintf("v%v.%v.%v", v.GetMajor(), v.GetMinor(), v.GetPatch())
		if s := v.GetSuffix(); s != "" {
			protocVersion += "-" + s
		}
	}
	g.P("// \tprotoc-gen-web ", version)
	g.P("// \tprotoc         ", protocVersion)
	g.P("// source: ", file.Desc.Path())
	g.P()
}

var service *protogen.Service //cur dealing service

func genService(file *protogen.File, g *protogen.GeneratedFile, s *protogen.Service) {
	service = s
	genPath(file, g)
	genClient(file, g)
	genServer(file, g)
}
func genPath(file *protogen.File, g *protogen.GeneratedFile) {
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		pathname := "_WebPath" + service.GoName + method.GoName
		pathurl := "/" + *file.Proto.Package + "." + string(service.Desc.Name()) + "/" + string(method.Desc.Name())
		g.P("var ", pathname, "=", strconv.Quote(pathurl))
	}
	g.P()
}
func genServer(file *protogen.File, g *protogen.GeneratedFile) {
	// Server interface.
	serverName := service.GoName + "WebServer"

	g.P("type ", serverName, " interface {")
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		g.P(method.Comments.Leading,
			method.GoName, "(", g.QualifiedGoIdent(contextPackage.Ident("Context")), ",*", g.QualifiedGoIdent(method.Input.GoIdent), ")(*", g.QualifiedGoIdent(method.Output.GoIdent), ",error)",
			method.Comments.Trailing)
	}
	g.P("}")
	g.P()
	// Server handler
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		fname := "func _" + service.GoName + "_" + method.GoName + "_" + "WebHandler"
		p1 := "handler func (" + g.QualifiedGoIdent(contextPackage.Ident("Context")) + ",*" + g.QualifiedGoIdent(method.Input.GoIdent) + ")(*" + g.QualifiedGoIdent(method.Output.GoIdent) + ",error)"
		freturn := g.QualifiedGoIdent(webPackage.Ident("OutsideHandler"))
		g.P(fname, "(", p1, ")", freturn, "{")
		g.P("return func(ctx *"+g.QualifiedGoIdent(webPackage.Ident("Context")), "){")
		g.P("req:=new(", g.QualifiedGoIdent(method.Input.GoIdent), ")")
		g.P("if ", g.QualifiedGoIdent(stringsPackage.Ident("HasPrefix")), "(ctx.GetContentType(),", strconv.Quote("application/json"), "){")
		g.P("data,e:=ctx.GetBody()")
		g.P("if e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusInternalServerError")), ",", g.QualifiedGoIdent(errorPackage.Ident("ConvertStdError")), "(e))")
		g.P("return")
		g.P("}")
		g.P("if len(data)>0{")
		g.P("e:=", g.QualifiedGoIdent(protojsonPackage.Ident("UnmarshalOptions{DiscardUnknown: true}")), ".Unmarshal(data,req)")
		g.P("if e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
		g.P("return")
		g.P("}")
		g.P("}")
		g.P("}else if ", g.QualifiedGoIdent(stringsPackage.Ident("HasPrefix")), "(ctx.GetContentType(),", strconv.Quote("application/x-protobuf"), "){")
		g.P("data,e:=ctx.GetBody()")
		g.P("if e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusInternalServerError")), ",", g.QualifiedGoIdent(errorPackage.Ident("ConvertStdError")), "(e))")
		g.P("return")
		g.P("}")
		g.P("if len(data)>0{")
		g.P("if e:=", g.QualifiedGoIdent(protoPackage.Ident("Unmarshal")), "(data,req);e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
		g.P("return")
		g.P("}")
		g.P("}")
		g.P("}else{")
		g.P("if e:=ctx.ParseForm();e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
		g.P("return")
		g.P("}")
		g.P("data:=", g.QualifiedGoIdent(bufpoolPackage.Ident("GetBuffer()")))
		g.P("defer ", g.QualifiedGoIdent(bufpoolPackage.Ident("PutBuffer(data)")))
		g.P("data.AppendByte('{')")
		for i, field := range method.Input.Fields {
			fname := string(field.Desc.Name())
			g.P("data.AppendString(", strconv.Quote(strconv.Quote(fname)+":"), ")")
			switch field.Desc.Kind() {
			case protoreflect.BoolKind:
				g.P("if form:=ctx.GetForm(", strconv.Quote(fname), ");len(form)==0{")
				if field.Desc.IsList() {
					g.P("data.AppendString(", strconv.Quote("null"), ")")
				} else {
					g.P("data.AppendString(", strconv.Quote("false"), ")")
				}
				g.P("}else{")
				g.P("data.AppendString(form)")
				g.P("}")
			case protoreflect.EnumKind:
				fallthrough
			case protoreflect.Int32Kind:
				fallthrough
			case protoreflect.Sint32Kind:
				fallthrough
			case protoreflect.Uint32Kind:
				fallthrough
			case protoreflect.Int64Kind:
				fallthrough
			case protoreflect.Sint64Kind:
				fallthrough
			case protoreflect.Uint64Kind:
				fallthrough
			case protoreflect.Sfixed32Kind:
				fallthrough
			case protoreflect.Fixed32Kind:
				fallthrough
			case protoreflect.FloatKind:
				fallthrough
			case protoreflect.Sfixed64Kind:
				fallthrough
			case protoreflect.Fixed64Kind:
				fallthrough
			case protoreflect.DoubleKind:
				g.P("if form:=ctx.GetForm(", strconv.Quote(fname), ");len(form)==0{")
				if field.Desc.IsList() {
					g.P("data.AppendString(", strconv.Quote("null"), ")")
				} else {
					g.P("data.AppendString(", strconv.Quote("0"), ")")
				}
				g.P("}else{")
				g.P("data.AppendString(form)")
				g.P("}")
			case protoreflect.StringKind:
				fallthrough
			case protoreflect.BytesKind:
				g.P("if form:=ctx.GetForm(", strconv.Quote(fname), ");len(form)==0{")
				if field.Desc.IsList() {
					g.P("data.AppendString(", strconv.Quote("null"), ")")
					g.P("}else{")
					g.P("data.AppendString(form)")
					g.P("}")
				} else {
					g.P("data.AppendString(", strconv.Quote("\"\""), ")")
					g.P("}else if len(form)<2 || form[0] !='\"' || form[len(form)-1]!='\"'{")
					g.P("data.AppendByte('\"')")
					g.P("data.AppendString(form)")
					g.P("data.AppendByte('\"')")
					g.P("}else{")
					g.P("data.AppendString(form)")
					g.P("}")
				}
			case protoreflect.MessageKind:
				g.P("if form:=ctx.GetForm(", strconv.Quote(fname), ");len(form)==0{")
				g.P("data.AppendString(", strconv.Quote("null"), ")")
				g.P("}else{")
				g.P("data.AppendString(form)")
				g.P("}")
			}
			if i != len(method.Input.Fields)-1 {
				g.P("data.AppendByte(',')")
			}
		}
		g.P("data.AppendByte('}')")
		g.P("if data.Len()>2{")
		g.P("e:=", g.QualifiedGoIdent(protojsonPackage.Ident("UnmarshalOptions{DiscardUnknown: true}")), ".Unmarshal(data.Bytes(),req)")
		g.P("if e!=nil{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
		g.P("return")
		g.P("}")
		g.P("}")
		g.P("}")

		pathurl := "/" + *file.Proto.Package + "." + string(service.Desc.Name()) + "/" + string(method.Desc.Name())
		//check
		if pbex.NeedCheck(method.Input) {
			g.P("if errstr := req.Validate(); errstr != \"\"{")
			g.P(g.QualifiedGoIdent(logPackage.Ident("Error")), "(ctx,\"[", pathurl, "]\",errstr)")
			g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), ")")
			g.P("return")
			g.P("}")
		}

		g.P("resp,e:=handler(ctx,req)")
		g.P("ee := ", g.QualifiedGoIdent(errorPackage.Ident("ConvertStdError")), "(e)")
		g.P("if ee!=nil{")
		g.P("if ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusBadRequest")), ",ee)")
		g.P("}else if ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(contextPackage.Ident("DeadlineExceeded")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusGatewayTimeout")), ",ee)")
		g.P("}else if ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(errorPackage.Ident("ErrAuth")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusUnauthorized")), ",ee)")
		g.P("}else if  ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(errorPackage.Ident("ErrBan")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusForbidden")), ",ee)")
		g.P("}else if ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(errorPackage.Ident("ErrLimit")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusServiceUnavailable")), ",ee)")
		g.P("}else if ", g.QualifiedGoIdent(errorPackage.Ident("Equal")), "(ee,", g.QualifiedGoIdent(errorPackage.Ident("ErrNotExist")), "){")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusNotFound")), ",ee)")
		g.P("}else{")
		g.P("ctx.Abort(", g.QualifiedGoIdent(httpPackage.Ident("StatusInternalServerError")), ",ee)")
		g.P("}")
		g.P("return")
		g.P("}")
		g.P("if resp == nil{")
		g.P("resp = new(", g.QualifiedGoIdent(method.Output.GoIdent), ")")
		g.P("}")
		g.P("if ", stringsPackage.Ident("HasPrefix"), "(ctx.GetAcceptType(),", strconv.Quote("application/x-protobuf"), "){")
		g.P("respd,_:=", g.QualifiedGoIdent(protoPackage.Ident("Marshal")), "(resp)")
		g.P("ctx.SetHeader(", strconv.Quote("Content-Type"), ",", strconv.Quote("application/x-protobuf"), ")")
		g.P("ctx.Write(respd)")
		g.P("}else{")
		g.P("respd,_:=", g.QualifiedGoIdent(protojsonPackage.Ident("MarshalOptions")), "{UseProtoNames: true, UseEnumNumbers: true, EmitUnpopulated: true}.Marshal(resp)")
		g.P("ctx.SetHeader(", strconv.Quote("Content-Type"), ",", strconv.Quote("application/json"), ")")
		g.P("ctx.Write(respd)")
		g.P("}")
		g.P("}")
		g.P("}")
	}

	//Server Register
	g.P("func Register", serverName, "(engine *", g.QualifiedGoIdent(webPackage.Ident("WebServer")), ",svc ", serverName, ",allmids map[string]", g.QualifiedGoIdent(webPackage.Ident("OutsideHandler")), "){")
	g.P("//avoid lint")
	g.P("_=allmids")
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		var timeout time.Duration
		if proto.HasExtension(mop, pbex.E_Timeout) {
			timeoutstr := proto.GetExtension(mop, pbex.E_Timeout).(string)
			var e error
			if timeout, e = time.ParseDuration(timeoutstr); e != nil {
				panic(fmt.Sprintf("method: %s in service: %s with timeout: %s format error:%s", method.Desc.Name(), service.Desc.Name(), timeoutstr, e))
			}
		}
		var mids []string
		if proto.HasExtension(mop, pbex.E_WebMidwares) {
			mids = proto.GetExtension(mop, pbex.E_WebMidwares).([]string)
		}
		fname := "_" + service.GoName + "_" + method.GoName + "_" + "WebHandler(svc." + method.GoName + ")"
		pathname := "_WebPath" + service.GoName + method.GoName
		if len(mids) > 0 {
			g.P("{")
			str := ""
			for _, mid := range mids {
				str += ","
				str += strconv.Quote(mid)
			}
			str = str[1:]
			g.P("requiredMids:=[]string{", str, "}")
			g.P("mids:=make([]", g.QualifiedGoIdent(webPackage.Ident("OutsideHandler")), ",0,", len(mids)+1, ")")
			g.P("for _,v:=range requiredMids{")
			g.P("if mid,ok:=allmids[v];ok{")
			g.P("mids = append(mids,mid)")
			g.P("}else{")
			g.P("panic(\"missing midware:\"+v)")
			g.P("}")
			g.P("}")
			g.P("mids = append(mids,", fname, ")")
			switch httpmetohd {
			case http.MethodGet:
				g.P("engine.Get(", pathname, ",", timeout.Nanoseconds(), ",mids...)")
			case http.MethodDelete:
				g.P("engine.Delete(", pathname, ",", timeout.Nanoseconds(), ",mids...)")
			case http.MethodPost:
				g.P("engine.Post(", pathname, ",", timeout.Nanoseconds(), ",mids...)")
			case http.MethodPut:
				g.P("engine.Put(", pathname, ",", timeout.Nanoseconds(), ",mids...)")
			case http.MethodPatch:
				g.P("engine.Patch(", pathname, ",", timeout.Nanoseconds(), ",mids...)")
			}
			g.P("}")
		} else {
			switch httpmetohd {
			case http.MethodGet:
				g.P("engine.Get(", pathname, ",", timeout.Nanoseconds(), ",", fname, ")")
			case http.MethodDelete:
				g.P("engine.Delete(", pathname, ",", timeout.Nanoseconds(), ",", fname, ")")
			case http.MethodPost:
				g.P("engine.Post(", pathname, ",", timeout.Nanoseconds(), ",", fname, ")")
			case http.MethodPut:
				g.P("engine.Put(", pathname, ",", timeout.Nanoseconds(), ",", fname, ")")
			case http.MethodPatch:
				g.P("engine.Patch(", pathname, ",", timeout.Nanoseconds(), ",", fname, ")")
			}
		}
	}
	g.P("}")
}

func genClient(file *protogen.File, g *protogen.GeneratedFile) {
	// Client interface.
	clientName := service.GoName + "WebClient"
	lowclientName := strings.ToLower(clientName[:1]) + clientName[1:]

	g.P("type ", clientName, " interface {")
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		p1 := g.QualifiedGoIdent(contextPackage.Ident("Context"))
		p2 := g.QualifiedGoIdent(method.Input.GoIdent)
		p3 := g.QualifiedGoIdent(httpPackage.Ident("Header"))
		r := g.QualifiedGoIdent(method.Output.GoIdent)
		g.P(method.Comments.Leading,
			method.GoName, "(", p1, ",*", p2, ",", p3, ")(*", r, ",error)",
			method.Comments.Trailing)
	}
	g.P("}")
	g.P()
	g.P("type ", lowclientName, " struct{")
	g.P("cc *", g.QualifiedGoIdent(webPackage.Ident("WebClient")))
	g.P("}")
	g.P("func New", clientName, "(c *", g.QualifiedGoIdent(webPackage.Ident("WebClient")), ")(", clientName, "){")
	g.P("return &", lowclientName, "{cc:c}")
	g.P("}")
	g.P()
	// Client handler
	for _, method := range service.Methods {
		mop := method.Desc.Options().(*descriptorpb.MethodOptions)
		if mop.GetDeprecated() {
			continue
		}
		if !proto.HasExtension(mop, pbex.E_Method) {
			continue
		}
		httpmetohd := strings.ToUpper(proto.GetExtension(mop, pbex.E_Method).(string))
		if httpmetohd != http.MethodGet && httpmetohd != http.MethodPost && httpmetohd != http.MethodPut && httpmetohd != http.MethodDelete && httpmetohd != http.MethodPatch {
			panic(fmt.Sprintf("method: %s in service: %s with not supported httpmetohd: %s", method.Desc.Name(), service.Desc.Name(), httpmetohd))
		}
		pathname := "_WebPath" + service.GoName + method.GoName
		p1 := "ctx " + g.QualifiedGoIdent(contextPackage.Ident("Context"))
		p2 := "req *" + g.QualifiedGoIdent(method.Input.GoIdent)
		p3 := "header " + g.QualifiedGoIdent(httpPackage.Ident("Header"))
		freturn := "(*" + g.QualifiedGoIdent(method.Output.GoIdent) + ",error)"
		g.P("func (c *", lowclientName, ")", method.GoName, "(", p1, ",", p2, ",", p3, ")", freturn, "{")
		g.P("if req == nil {")
		g.P("return nil,", g.QualifiedGoIdent(errorPackage.Ident("ErrReq")))
		g.P("}")

		g.P("if header == nil {")
		g.P("header = make(", g.QualifiedGoIdent(httpPackage.Ident("Header")), ")")
		g.P("}")

		var timeout time.Duration
		if proto.HasExtension(mop, pbex.E_Timeout) {
			timeoutstr := proto.GetExtension(mop, pbex.E_Timeout).(string)
			if timeoutstr != "" {
				var e error
				timeout, e = time.ParseDuration(timeoutstr)
				if e != nil {
					panic(fmt.Sprintf("method: %s in service: %s with timeout: %s format error:%s", method.Desc.Name(), service.Desc.Name(), timeoutstr, e))
				}
			}
		}
		if httpmetohd == http.MethodGet || httpmetohd == http.MethodDelete {
			g.P("header.Set(", strconv.Quote("Content-Type"), ",", strconv.Quote("application/x-www-form-urlencoded"), ")")
			g.P("header.Set(", strconv.Quote("Accept"), ",", strconv.Quote("application/x-protobuf"), ")")
			g.P("query :=", g.QualifiedGoIdent(bufpoolPackage.Ident("GetBuffer")), "()")
			g.P("defer ", g.QualifiedGoIdent(bufpoolPackage.Ident("PutBuffer")), "(query)")
			for _, field := range method.Input.Fields {
				fname := string(field.Desc.Name())
				switch field.Desc.Kind() {
				case protoreflect.BoolKind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendBools(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendBool(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.EnumKind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendByte('[')")
						g.P("for i,e:=range req.", field.GoName, "{")
						g.P("query.AppendInt32(int32(e))")
						g.P("if i == len(req.", field.GoName, ")-1{")
						g.P("query.AppendByte(']')")
						g.P("}else{")
						g.P("query.AppendByte(',')")
						g.P("}")
						g.P("}")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendInt32(int32(req.", field.GoName, "))")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.Sfixed32Kind:
					fallthrough
				case protoreflect.Sint32Kind:
					fallthrough
				case protoreflect.Int32Kind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendInt32s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendInt32(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.Sfixed64Kind:
					fallthrough
				case protoreflect.Sint64Kind:
					fallthrough
				case protoreflect.Fixed32Kind:
					fallthrough
				case protoreflect.Int64Kind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendInt64s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendInt64(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.Uint32Kind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendUint32s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendUint32(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.Fixed64Kind:
					fallthrough
				case protoreflect.Uint64Kind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendUint64s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendUint64(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.FloatKind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendFloat32s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendFloat32(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.DoubleKind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendFloat64s(req.", field.GoName, ")")
					} else {
						g.P("if req.", field.GoName, "!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendFloat64(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.StringKind:
					if field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendStrings(req.", field.GoName, ")")
					} else {
						g.P("if len(req.", field.GoName, ")!=0{")
						g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
						g.P("query.AppendString(req.", field.GoName, ")")
					}
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.BytesKind:
					g.P("if len(req.", field.GoName, ")!=0{")
					g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
					g.P("temp,_:=", g.QualifiedGoIdent(stdjsonPackage.Ident("Marshal")), "(req.", field.GoName, ")")
					g.P("query.AppendByteSlice(temp)")
					g.P("query.AppendByte('&')")
					g.P("}")
				case protoreflect.MessageKind:
					if field.Desc.IsMap() || field.Desc.IsList() {
						g.P("if len(req.", field.GoName, ")!=0{")
					} else {
						g.P("if req.", field.GoName, "!=nil{")
					}
					g.P("query.AppendString(", strconv.Quote(fname+"="), ")")
					g.P("temp,_:=", g.QualifiedGoIdent(stdjsonPackage.Ident("Marshal")), "(req.", field.GoName, ")")
					g.P("query.AppendByteSlice(temp)")
					g.P("query.AppendByte('&')")
					g.P("}")
				}
			}
			switch httpmetohd {
			case http.MethodGet:
				g.P("r,e:=c.cc.Get(ctx,", timeout.Nanoseconds(), ",", pathname, ",query.String(),header,", g.QualifiedGoIdent(metadataPackage.Ident("GetMetadata")), "(ctx))")
			case http.MethodDelete:
				g.P("r,e:=c.cc.Delete(ctx,", timeout.Nanoseconds(), ",", pathname, ",query.String(),header,", g.QualifiedGoIdent(metadataPackage.Ident("GetMetadata")), "(ctx))")
			}
		} else {
			g.P("header.Set(", strconv.Quote("Content-Type"), ",", strconv.Quote("application/x-protobuf"), ")")
			g.P("header.Set(", strconv.Quote("Accept"), ",", strconv.Quote("application/x-protobuf"), ")")
			g.P("reqd,_:=", g.QualifiedGoIdent(protoPackage.Ident("Marshal")), "(req)")
			switch httpmetohd {
			case http.MethodPost:
				g.P("r,e:=c.cc.Post(ctx,", timeout.Nanoseconds(), ",", pathname, ",\"\",header,", g.QualifiedGoIdent(metadataPackage.Ident("GetMetadata")), "(ctx),reqd)")
			case http.MethodPut:
				g.P("r,e:=c.cc.Put(ctx,", timeout.Nanoseconds(), ",", pathname, ",\"\",header,", g.QualifiedGoIdent(metadataPackage.Ident("GetMetadata")), "(ctx),reqd)")
			case http.MethodPatch:
				g.P("r,e:=c.cc.Patch(ctx,", timeout.Nanoseconds(), ",", pathname, ",\"\",header,", g.QualifiedGoIdent(metadataPackage.Ident("GetMetadata")), "(ctx),reqd)")
			}
		}
		g.P("if e != nil {")
		g.P("return nil,e")
		g.P("}")
		g.P("defer r.Body.Close()")
		g.P("data,e:=", g.QualifiedGoIdent(ioPackage.Ident("ReadAll")), "(r.Body)")
		g.P("if e!=nil {")
		g.P("return nil,e")
		g.P("}")
		g.P("resp := new(", g.QualifiedGoIdent(method.Output.GoIdent), ")")
		g.P("if len(data)==0{")
		g.P("return resp,nil")
		g.P("}")
		g.P("if e:=", g.QualifiedGoIdent(protoPackage.Ident("Unmarshal")), "(data,resp);e!=nil{")
		g.P("return nil,", g.QualifiedGoIdent(errorPackage.Ident("ErrResp")))
		g.P("}")
		g.P("return resp, nil")
		g.P("}")
	}
}
