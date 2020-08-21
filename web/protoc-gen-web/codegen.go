package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func genfile(plugin *protogen.Plugin, file *protogen.File) {
	f := plugin.NewGeneratedFile(file.GeneratedFilenamePrefix+".web.go", file.GoImportPath)
	//package
	dealPackage(f, file)
	//import
	dealImports(f, file)
	//instance
	dealInstance(f, file)
	//interface
	dealInterface(f, file)
	//handlefunc
	dealHandlefunc(f, file)
	//path
	dealPath(f, file)
	//server
	dealServer(f, file)
}
func dealPackage(f *protogen.GeneratedFile, file *protogen.File) {
	f.P(fmt.Sprintf("package %s", file.GoPackageName))
	f.P()
}
func dealImports(f *protogen.GeneratedFile, file *protogen.File) {
	deal := func(imports map[string]struct{}) {
		if len(imports) == 0 {
			return
		}
		temp := make([]string, len(imports))
		count := 0
		for v := range imports {
			temp[count] = v
			count++
		}
		sort.Strings(temp)
		for _, v := range temp {
			f.P(fmt.Sprintf("%s", v))
		}
	}
	//std package
	importsSTD := make(map[string]struct{})
	//importsSTD[strconv.Quote("fmt")] = struct{}{}
	importsSTD[strconv.Quote("encoding/json")] = struct{}{}
	importsSTD[strconv.Quote("net/http")] = struct{}{}
	importsSTD[strconv.Quote("net/url")] = struct{}{}
	importsSTD[strconv.Quote("strings")] = struct{}{}

	//third package
	importsTHIRD := make(map[string]struct{})
	importsTHIRD[strconv.Quote("github.com/chenjie199234/Corelib/web")] = struct{}{}
	importsTHIRD[strconv.Quote("google.golang.org/protobuf/proto")] = struct{}{}
	//self package
	importsSELF := make(map[string]struct{})
	for _, service := range file.Services {
		for _, method := range service.Methods {
			if method.Input.GoIdent.GoImportPath.String() != "" && method.Input.GoIdent.GoImportPath.String() != file.GoImportPath.String() {
				importsSELF[method.Input.GoIdent.GoImportPath.String()] = struct{}{}
			}
			if method.Output.GoIdent.GoImportPath.String() != "" && method.Input.GoIdent.GoImportPath.String() != file.GoImportPath.String() {
				importsSELF[method.Output.GoIdent.GoImportPath.String()] = struct{}{}
			}
		}
	}
	f.P("import (")
	f.P("//std")
	deal(importsSTD)
	f.P()
	f.P("//third")
	deal(importsTHIRD)
	f.P()
	f.P("//project")
	deal(importsSELF)
	f.P(")")
	f.P()
}
func dealInstance(f *protogen.GeneratedFile, file *protogen.File) {
	for _, service := range file.Services {
		instancename := fmt.Sprintf("web%sInstance", service.GoName)
		interfacename := fmt.Sprintf("web%s", service.GoName)
		f.P(fmt.Sprintf("var %s %s", instancename, interfacename))
		f.P()
	}
}
func dealInterface(f *protogen.GeneratedFile, file *protogen.File) {
	for _, service := range file.Services {
		interfacename := fmt.Sprintf("web%s", service.GoName)
		f.P(fmt.Sprintf("type %s interface {", interfacename))
		f.P("RegisterMidware() map[string][]web.OutsideHandler")
		for _, method := range service.Methods {
			inputpac := method.Input.Desc.(protoreflect.Descriptor).ParentFile().Package()
			outputpac := method.Output.Desc.(protoreflect.Descriptor).ParentFile().Package()
			var input string
			var output string
			if string(inputpac) == string(file.GoPackageName) {
				input = fmt.Sprintf("%s", method.Input.GoIdent.GoName)
			} else {
				input = fmt.Sprintf("%s.%s", inputpac, method.Input.GoIdent.GoName)
			}
			if string(outputpac) == string(file.GoPackageName) {
				output = fmt.Sprintf("%s", method.Output.GoIdent.GoName)
			} else {
				output = fmt.Sprintf("%s.%s", outputpac, method.Output.GoIdent.GoName)
			}
			f.P(fmt.Sprintf("%s(*web.Context, *%s) (*%s,error)", method.GoName, input, output))
		}
		f.P("}")
		f.P()
	}
}
func dealHandlefunc(f *protogen.GeneratedFile, file *protogen.File) {
	for _, service := range file.Services {
		instancename := fmt.Sprintf("web%sInstance", service.GoName)
		interfacename := fmt.Sprintf("web%s", service.GoName)
		for _, method := range service.Methods {
			handlefuncname := fmt.Sprintf("deal%s%s", interfacename, method.GoName)
			inputpac := method.Input.Desc.(protoreflect.Descriptor).ParentFile().Package()
			f.P(fmt.Sprintf("func %s(ctx *web.Context) {", handlefuncname))
			if string(inputpac) == string(file.GoPackageName) {
				f.P(fmt.Sprintf("req := &%s{}", method.Input.GoIdent.GoName))
			} else {
				f.P(fmt.Sprintf("req := &%s.%s{}", inputpac, method.Input.GoIdent.GoName))
			}
			f.P("switch ctx.GetContentType() {")
			f.P("case \"\":fallthrough")
			f.P("case \"application/x-www-form-urlencoded\":")
			f.P("encoded := false")
			f.P("if data := ctx.GetForm(\"encode\"); data == \"1\" {")
			f.P("encoded = true")
			f.P("}")
			f.P("if data := ctx.GetForm(\"json\"); data != \"\" {")
			f.P("var e error")
			f.P("if encoded {")
			f.P("if data, e = url.QueryUnescape(data); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"encoded request data format error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("}")
			f.P("if e = json.Unmarshal(web.Str2byte(data), req); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"decode json request data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("} else if data := ctx.GetForm(\"proto\"); data != \"\" {")
			f.P("var e error")
			f.P("if encoded {")
			f.P("if data, e = url.QueryUnescape(data); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"encoded request data format error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("}")
			f.P("if e = proto.Unmarshal(web.Str2byte(data), req); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"decode proto request data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("}")
			f.P("case \"application/json\":")
			f.P("if body, _ := ctx.GetBody(); len(body) > 0 {")
			f.P("if e := json.Unmarshal(body, req); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"decode json request data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("}")
			f.P("case \"application/x-protobuf\":")
			f.P("if body, _ := ctx.GetBody(); len(body) > 0 {")
			f.P("if e := proto.Unmarshal(body, req); e != nil {")
			f.P("ctx.WriteString(http.StatusBadRequest, \"decode proto request data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("}")
			f.P("default:")
			f.P("ctx.WriteString(http.StatusBadRequest, \"unknown Content-Type:\"+ctx.GetContentType())")
			f.P("return")
			f.P("}")
			f.P(fmt.Sprintf("resp,e:=%s.%s(ctx,req)", instancename, method.GoName))
			f.P("if e!=nil{")
			f.P("ctx.WriteString(http.StatusInternalServerError, e.Error())")
			f.P("return")
			f.P("}")
			f.P("if strings.Contains(ctx.GetAcceptType(), \"application/x-protobuf\") {")
			f.P("data, e := proto.Marshal(resp)")
			f.P("if e != nil {")
			f.P("ctx.WriteString(http.StatusInternalServerError, \"encode proto response data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("ctx.Write(http.StatusOK, data)")
			f.P("} else {")
			f.P("data, e := json.Marshal(resp)")
			f.P("if e != nil {")
			f.P("ctx.WriteString(http.StatusInternalServerError, \"encode json response data error:\"+e.Error())")
			f.P("return")
			f.P("}")
			f.P("ctx.Write(http.StatusOK, data)")
			f.P("}")
			f.P("}")
		}
	}
}
func dealPath(f *protogen.GeneratedFile, file *protogen.File) {
	for _, service := range file.Services {
		for _, method := range service.Methods {
			pathname := fmt.Sprintf("Path%s%s", service.GoName, method.GoName)
			path := fmt.Sprintf("/%s/%s", service.GoName, method.GoName)
			f.P(fmt.Sprintf("var %s = %s", pathname, strconv.Quote(path)))
		}
		f.P()
	}
}
func dealServer(f *protogen.GeneratedFile, file *protogen.File) {
	for _, service := range file.Services {
		instancename := fmt.Sprintf("web%sInstance", service.GoName)
		interfacename := fmt.Sprintf("web%s", service.GoName)
		f.P(fmt.Sprintf("func Register%s(engine *web.Web, instance %s) {", interfacename, interfacename))
		f.P(fmt.Sprintf("%s = instance", instancename))
		f.P("pathmids := instance.RegisterMidware()")
		for _, method := range service.Methods {
			handlefuncname := fmt.Sprintf("deal%s%s", interfacename, method.GoName)
			pathname := fmt.Sprintf("Path%s%s", service.GoName, method.GoName)
			f.P(fmt.Sprintf("if mids, ok := pathmids[%s]; ok && len(mids) > 0 {", pathname))
			switch strings.ToUpper(strings.TrimSuffix(string(method.Comments.Trailing), "\n")) {
			case "":
				fallthrough
			case http.MethodGet:
				f.P(fmt.Sprintf("engine.GET(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodPost:
				f.P(fmt.Sprintf("engine.POST(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodPatch:
				f.P(fmt.Sprintf("engine.PATCH(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodPut:
				f.P(fmt.Sprintf("engine.PUT(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodDelete:
				f.P(fmt.Sprintf("engine.DELETE(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodHead:
				f.P(fmt.Sprintf("engine.HEAD(%s,append(mids,%s)...)", pathname, handlefuncname))
			case http.MethodOptions:
				f.P(fmt.Sprintf("engine.OPTIONS(%s,append(mids,%s)...)", pathname, handlefuncname))
			default:
				panic("unknown http method:" + strings.ToUpper(strings.TrimSuffix(string(method.Comments.Trailing), "\n")))
			}
			f.P("} else {")
			switch strings.ToUpper(strings.TrimSuffix(string(method.Comments.Trailing), "\n")) {
			case "":
				fallthrough
			case http.MethodGet:
				f.P(fmt.Sprintf("engine.GET(%s,%s)", pathname, handlefuncname))
			case http.MethodPost:
				f.P(fmt.Sprintf("engine.POST(%s,%s)", pathname, handlefuncname))
			case http.MethodPatch:
				f.P(fmt.Sprintf("engine.PATCH(%s,%s)", pathname, handlefuncname))
			case http.MethodPut:
				f.P(fmt.Sprintf("engine.PUT(%s,%s)", pathname, handlefuncname))
			case http.MethodDelete:
				f.P(fmt.Sprintf("engine.DELETE(%s,%s)", pathname, handlefuncname))
			case http.MethodHead:
				f.P(fmt.Sprintf("engine.HEAD(%s,%s)", pathname, handlefuncname))
			case http.MethodOptions:
				f.P(fmt.Sprintf("engine.OPTIONS(%s,%s)", pathname, handlefuncname))
			default:
				panic("unknown http method:" + strings.ToUpper(strings.TrimSuffix(string(method.Comments.Trailing), "\n")))
			}
			f.P("}")
		}
		f.P("}")
		f.P()
	}
}
