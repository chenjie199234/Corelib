package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/chenjie199234/Corelib/codegen/tml/api"
	statusapi "github.com/chenjie199234/Corelib/codegen/tml/api/status"
	subapi "github.com/chenjie199234/Corelib/codegen/tml/api/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/cmd"
	"github.com/chenjie199234/Corelib/codegen/tml/config"
	"github.com/chenjie199234/Corelib/codegen/tml/configfile"
	"github.com/chenjie199234/Corelib/codegen/tml/dao"
	subdao "github.com/chenjie199234/Corelib/codegen/tml/dao/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/deploy"
	"github.com/chenjie199234/Corelib/codegen/tml/ecode"
	"github.com/chenjie199234/Corelib/codegen/tml/git"
	"github.com/chenjie199234/Corelib/codegen/tml/gomod"
	"github.com/chenjie199234/Corelib/codegen/tml/html"
	"github.com/chenjie199234/Corelib/codegen/tml/mainfile"
	"github.com/chenjie199234/Corelib/codegen/tml/model"
	submodel "github.com/chenjie199234/Corelib/codegen/tml/model/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/readme"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xcrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xgrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xraw"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xweb"
	"github.com/chenjie199234/Corelib/codegen/tml/service"
	serviceraw "github.com/chenjie199234/Corelib/codegen/tml/service/raw"
	servicestatus "github.com/chenjie199234/Corelib/codegen/tml/service/status"
	"github.com/chenjie199234/Corelib/codegen/tml/service/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/util"
	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/name"
	cname "github.com/chenjie199234/Corelib/util/name"
)

var ver = flag.Bool("v", false, "version info")
var appname = flag.String("n", "", "app name\ncharacter:[a-z][0-9]\nfirst character must in [a-z]")
var packagename = flag.String("p", "", "package name\nmust be app name or end with app name\nif this is empty the app name will be used as the package name\nthis is useful when your project will be uploaded to github or gitlab\ne.g. github.com/path_to_the_repo/app_name")

var gensub = flag.String("sub", "", "create subservice in this app\ncharacter:[a-z][0-9]\nfirst character must in [a-z]\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genkube = flag.Bool("kube", false, "create project's kubernetes config file\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genhtml = flag.Bool("html", false, "create project's html template\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")

func main() {
	flag.Parse()
	if *ver {
		fmt.Println(version.String())
		return
	}
	check()
	step := 0
	if _, e := os.Stat("./go.mod"); e != nil && !os.IsNotExist(e) {
		panic("get ./go.mod info error: " + e.Error())
	} else if e != nil {
		//create base project
		if finfo, e := os.Stat("./" + *appname); e != nil {
			if !os.IsNotExist(e) {
				panic("get ./" + *appname + " info error: " + e.Error())
			}
			if e := os.MkdirAll("./"+*appname, 0755); e != nil {
				panic("mkdir ./" + *appname + " error: " + e.Error())
			}
		} else if !finfo.IsDir() {
			panic("./" + *appname + " exist and it is not a dir")
		} else if files, e := os.ReadDir("./" + *appname); e != nil {
			panic("./" + *appname + " check dir empty error: " + e.Error())
		} else if len(files) > 0 {
			panic("./" + *appname + " exist and it is not an empty dir")
		}
		if e = os.Chdir("./" + *appname); e != nil {
			panic("cd ./" + *appname + " error: " + e.Error())
		}
		createBaseProject()
		step++
	}
	if len(*gensub) != 0 {
		if step > 0 {
			fmt.Println("=================================================================================================================")
		}
		createSubProject()
		step++
	}
	if *genkube {
		if step > 0 {
			fmt.Println("=================================================================================================================")
		}
		createKubernetes()
		step++
	}
	if *genhtml {
		if step > 0 {
			fmt.Println("=================================================================================================================")
		}
		createHtml()
	}
}
func check() {
	if e := cname.SingleCheck(*appname, false); e != nil {
		panic(e)
	} else {
		if *packagename == "" {
			packagename = appname
		}
		if *packagename != *appname && !strings.HasSuffix(*packagename, "/"+*appname) {
			panic("package's name must be app name or end with app name,e.g. github.com/path_to_the_repo/app_name")
		}
		if _, e := os.Stat("./go.mod"); e != nil && !os.IsNotExist(e) {
			panic("get ./go.mod info error: " + e.Error())
		} else if e != nil {
			//we are creating a new project
			return
		}
		//we are updating an exist project
		//need to do some check
		if len(*gensub) != 0 || *genhtml || *genkube {
			if data, e := os.ReadFile("./model/model.go"); e != nil {
				panic("read ./model/model.go error: " + e.Error())
			} else {
				lines := strings.Split(string(data), "\n")
				find := false
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if len(line) == 0 {
						continue
					}
					if line[len(line)-1] == '\r' {
						line = line[:len(line)-1]
					}
					if !strings.HasPrefix(line, "const Name = ") {
						continue
					}
					line = line[13:]
					if len(line) <= 2 || line[0] != '"' || line[len(line)-1] != '"' {
						panic("./model/model.go broken!")
					}
					line = line[1 : len(line)-1]
					if line != *appname {
						panic("app name conflict,this is not the required app")
					}
					find = true
					break
				}
				if !find {
					panic("./model/model.go broken!")
				}
			}
			if data, e := os.ReadFile("./go.mod"); e != nil {
				panic("read ./go.mod error: " + e.Error())
			} else {
				lines := strings.Split(string(data), "\n")
				find := false
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if len(line) == 0 {
						continue
					}
					if line[len(line)-1] == '\r' {
						line = line[:len(line)-1]
					}
					if !strings.HasPrefix(line, "module") {
						continue
					}
					line = strings.TrimSpace(line[6:])
					if line != *packagename {
						panic("package name conflict,this is not the required package")
					}
					find = true
					break
				}
				if !find {
					panic("./go.mod broken!")
				}
			}
		}
	}
}
func createBaseProject() {
	fmt.Println("start create base app.")
	api.CreatePathAndFile()

	statusapi.CreatePathAndFile(*packagename, *appname)

	ecode.CreatePathAndFile()

	config.CreatePathAndFile(*packagename)

	configfile.CreatePathAndFile(*appname)

	dao.CreatePathAndFile(*packagename)

	subdao.CreatePathAndFile("status")
	subdao.CreatePathAndFile("raw")

	mainfile.CreatePathAndFile(*packagename)

	gomod.CreatePathAndFile(*packagename)

	model.CreatePathAndFile(*appname)
	submodel.CreatePathAndFile("status")
	submodel.CreatePathAndFile("raw")

	util.CreatePathAndFile()

	xcrpc.CreatePathAndFile(*packagename)

	xgrpc.CreatePathAndFile(*packagename)

	xraw.CreatePathAndFile(*packagename)

	xweb.CreatePathAndFile(*packagename)

	service.CreatePathAndFile(*packagename)

	servicestatus.CreatePathAndFile(*packagename)
	serviceraw.CreatePathAndFile(*packagename)

	cmd.CreatePathAndFile(*packagename, *appname)

	readme.CreatePathAndFile(*appname)

	git.CreatePathAndFile()

	fmt.Println("base app create success!")
}
func createSubProject() {
	fmt.Println("start create sub service.")
	if e := name.SingleCheck(*gensub, false); e != nil {
		panic(e)
	}
	_, e := os.Stat("./api/" + *appname + "_" + *gensub + ".proto")
	if e == nil {
		panic("./api/" + *appname + "_" + *gensub + ".proto already exist")
	}
	if !os.IsNotExist(e) {
		panic("./api/" + *appname + "_" + *gensub + ".proto check file exist error: " + e.Error())
	}
	_, e = os.Stat("./service/" + *gensub)
	if e == nil {
		panic("./service/" + *gensub + " already exist")
	}
	if !os.IsNotExist(e) {
		panic("./service/" + *gensub + " check dir exist error: " + e.Error())
	}
	_, e = os.Stat("./dao/" + *gensub)
	if e == nil {
		panic("./dao/" + *gensub + " already exist")
	}
	if !os.IsNotExist(e) {
		panic("./dao/" + *gensub + " check dir exist error: " + e.Error())
	}
	_, e = os.Stat("./model/" + *gensub + ".go")
	if e == nil {
		panic("./model/" + *gensub + ".go already exist")
	}
	if !os.IsNotExist(e) {
		panic("./model/" + *gensub + ".go check file exist error: " + e.Error())
	}
	//sub api
	subapi.CreatePathAndFile(*packagename, *appname, *gensub)
	//sub dao
	subdao.CreatePathAndFile(*gensub)
	//sub service
	sub.CreatePathAndFile(*packagename, *gensub)
	//sub model
	submodel.CreatePathAndFile(*gensub)
	updateSubProjectService()
	fmt.Println("sub service create success!")
}
func updateSubProjectService() {
	data, e := os.ReadFile("./service/service.go")
	if e != nil {
		panic("read ./service/service.go failed,error:" + e.Error())
	}
	tmpdata := bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'})
	windows := len(tmpdata) != len(data)
	pieces := bytes.Split(tmpdata, []byte{'\n'})
	find := false
	updated := false
	varname := "Svc" + string((*gensub)[0]-32) + (*gensub)[1:]
	for i := range pieces {
		tmp := bytes.TrimSpace(bytes.Clone(pieces[i]))
		//remove the comments
		tmp, _, _ = bytes.Cut(tmp, []byte{'/', '/'})
		for {
			if s := bytes.Index(tmp, []byte{'/', '*'}); s >= 0 {
				e := bytes.Index(tmp, []byte{'*', '/'})
				if e < s {
					break
				}
				tmp = append(tmp[:s], tmp[e+2:]...)
			} else {
				break
			}
		}
		if find {
			if bytes.Equal(tmp, []byte{')'}) {
				newpieces := make([][]byte, 0, len(pieces)+3)
				newpieces = append(newpieces, pieces[:i]...)
				newpieces = append(newpieces, []byte("\t\""+*packagename+"/service/"+*gensub+"\""))
				newpieces = append(newpieces, pieces[i])
				newpieces = append(newpieces, []byte{})
				newpieces = append(newpieces, []byte("var "+varname+" *"+*gensub+".Service"))
				newpieces = append(newpieces, pieces[i+1:]...)
				pieces = newpieces
				updated = true
				break
			}
		} else {
			if bytes.HasPrefix(tmp, []byte("import")) {
				find = bytes.HasSuffix(tmp, []byte{'('})
			}
		}
	}
	if !find || !updated {
		panic("./service/service.go broken,missing import ()")
	}
	find = false
	updated = false
	for i := range pieces {
		tmp := bytes.TrimSpace(bytes.Clone(pieces[i]))
		//remove the comments
		tmp, _, _ = bytes.Cut(tmp, []byte{'/', '/'})
		for {
			if s := bytes.Index(tmp, []byte{'/', '*'}); s >= 0 {
				e := bytes.Index(tmp, []byte{'*', '/'})
				if e < s {
					break
				}
				tmp = append(tmp[:s], tmp[e+2:]...)
			} else {
				break
			}
		}
		if find {
			if bytes.Equal(tmp, []byte("return nil")) {
				newpieces := make([][]byte, 0, len(pieces)+3)
				newpieces = append(newpieces, pieces[:i]...)
				newpieces = append(newpieces, []byte("\tif "+varname+", e = "+*gensub+".Start(); e != nil {"))
				newpieces = append(newpieces, []byte("\t\treturn e"))
				newpieces = append(newpieces, []byte("\t}"))
				newpieces = append(newpieces, pieces[i:]...)
				pieces = newpieces
				updated = true
				break
			}
		} else {
			if bytes.HasPrefix(tmp, []byte("func")) {
				find = bytes.HasPrefix(bytes.TrimSpace(tmp[4:]), []byte("StartService()"))
			}
		}
	}
	if !find || !updated {
		panic("./service/service.go broken,func StartService missing or broken")
	}
	find = false
	updated = false
	for i := range pieces {
		tmp := bytes.TrimSpace(bytes.Clone(pieces[i]))
		//remove the comments
		tmp, _, _ = bytes.Cut(tmp, []byte{'/', '/'})
		for {
			if s := bytes.Index(tmp, []byte{'/', '*'}); s >= 0 {
				e := bytes.Index(tmp, []byte{'*', '/'})
				if e < s {
					break
				}
				tmp = append(tmp[:s], tmp[e+2:]...)
			} else {
				break
			}
		}
		if bytes.HasPrefix(tmp, []byte("func")) {
			find = bytes.HasPrefix(bytes.TrimSpace(tmp[4:]), []byte("StopService()"))
		}
		if find {
			newpieces := make([][]byte, 0, len(pieces)+1)
			newpieces = append(newpieces, pieces[:i+1]...)
			newpieces = append(newpieces, []byte("\t"+varname+".Stop()"))
			newpieces = append(newpieces, pieces[i+1:]...)
			pieces = newpieces
			updated = true
			break
		}
	}
	if !find || !updated {
		panic("./service/service.go broken,func StopService missing or broken")
	}
	writer, e := os.OpenFile("./service/service.go", os.O_WRONLY|os.O_TRUNC, 0644)
	if e != nil {
		panic("write ./service/service.go failed,error:" + e.Error())
	}
	if windows {
		_, e = writer.Write(bytes.Join(pieces, []byte{'\r', '\n'}))
	} else {
		_, e = writer.Write(bytes.Join(pieces, []byte{'\n'}))
	}
	if e != nil {
		panic("write ./service/service.go failed,error:" + e.Error())
	}
}

/*
	func updateSubProjectService() {
		fset := token.NewFileSet()
		file, e := parser.ParseFile(fset, "./service/service.go", nil, parser.ParseComments)
		if e != nil {
			panic("./service/service.go parse failed,error:" + e.Error())
		}
		//import
		var importDecl *ast.GenDecl
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
				importDecl = gen
				break
			}
		}
		if importDecl == nil {
			panic("./service/service.go broken,missing import()")
		}
		newImport := &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: `"` + *packagename + "/service/" + *gensub + `"`,
			},
		}
		importDecl.Specs = append(importDecl.Specs, newImport)

		//var
		newVar := &ast.GenDecl{
			Lparen: token.NoPos,
			Rparen: token.NoPos,
			Tok:    token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:])},
					Type: &ast.StarExpr{
						X: &ast.SelectorExpr{
							X:   ast.NewIdent(*gensub),   // package name
							Sel: ast.NewIdent("Service"), // struct name
						},
					},
				},
			},
		}
		newdescls := make([]ast.Decl, 0, len(file.Decls)+1)
		for i, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && (gen.Tok == token.VAR || gen.Tok == token.TYPE || gen.Tok == token.CONST) {
				newdescls = append(newdescls, file.Decls[:i]...)
				newdescls = append(newdescls, newVar)
				newdescls = append(newdescls, file.Decls[i:]...)
				file.Decls = newdescls
				break
			}
			if _, ok := decl.(*ast.FuncDecl); ok {
				newdescls = append(newdescls, file.Decls[:i]...)
				newdescls = append(newdescls, newVar)
				newdescls = append(newdescls, file.Decls[i:]...)
				file.Decls = newdescls
				break
			}
		}
		//start
		var startDecl *ast.FuncDecl
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "StartService" {
				startDecl = f
				break
			}
		}
		if startDecl == nil || startDecl.Body == nil || len(startDecl.Body.List) == 0 {
			panic("./service/service.go broken,missing func StartService")
		}
		ifStmt := &ast.IfStmt{
			// Svcxxx,e := xxx.Start()
			Init: &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:]), ast.NewIdent("e")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(*gensub),
							Sel: ast.NewIdent("Start"),
						},
					},
				},
			},
			//e != nil
			Cond: &ast.BinaryExpr{
				X:  ast.NewIdent("e"),
				Op: token.NEQ,
				Y:  ast.NewIdent("nil"),
			},
			//return e
			Body: &ast.BlockStmt{
				List: []ast.Stmt{&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent("e")},
				}},
			},
		}
		startDecl.Body.List = append(startDecl.Body.List[:len(startDecl.Body.List)-1], ifStmt, startDecl.Body.List[len(startDecl.Body.List)-1])
		//stop
		var stopDecl *ast.FuncDecl
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "StopService" {
				stopDecl = f
				break
			}
		}
		if stopDecl == nil || stopDecl.Body == nil || len(stopDecl.Body.List) == 0 {
			panic("./service/service.go broken,missing func StopService")
		}
		stopStmt := &ast.ExprStmt{
			// Svcxxx.Stop()
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:]),
					Sel: ast.NewIdent("Stop"),
				},
			},
		}
		stopDecl.Body.List = append(stopDecl.Body.List, stopStmt)

		var buf bytes.Buffer
		if err := format.Node(&buf, fset, file); err != nil {
			panic(err)
		}
		if err := os.WriteFile("./service/service.go", buf.Bytes(), 0644); err != nil {
			panic(err)
		}
	}

	func updateSubProjectXweb() {
		fset := token.NewFileSet()
		file, e := parser.ParseFile(fset, "./server/xweb/xweb.go", nil, parser.ParseComments)
		if e != nil {
			panic("./server/xweb/xweb.go parse failed,error:" + e.Error())
		}
		var start *ast.FuncDecl
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "StartWebServer" {
				start = f
				break
			}
		}
		if start == nil || start.Body == nil || len(start.Body.List) == 0 {
			panic("./server/xweb/xweb.go broken,missing func StartWebServer")
		}
		registerStmt := &ast.ExprStmt{
			// api.RegisterxxxWebServer(r, service.Svcxxx, mids.AllMids())
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("api"),
					Sel: ast.NewIdent("Register" + string((*gensub)[0]-32) + (*gensub)[1:] + "WebServer"),
				},
				Args: []ast.Expr{
					ast.NewIdent("r"),
					&ast.SelectorExpr{
						X:   ast.NewIdent("service"),
						Sel: ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:]),
					},
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("mids"),
							Sel: ast.NewIdent("AllMids"),
						},
					},
				},
			},
		}
		find := false
		//insert the registerStmt before server.SetRouter(r)
		for i := len(start.Body.List) - 1; i >= 0; i-- {
			stmt := start.Body.List[i]
			exprStmt, ok := stmt.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := exprStmt.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if xIdent, ok := sel.X.(*ast.Ident); !ok || xIdent.Name != "server" {
				continue
			}
			if sel.Sel.Name != "SetRouter" {
				continue
			}
			find = true
			newlist := make([]ast.Stmt, 0, len(start.Body.List)+1)
			newlist = append(newlist, start.Body.List[:i]...)
			newlist = append(newlist, registerStmt)
			newlist = append(newlist, start.Body.List[i:]...)
			start.Body.List = newlist
			break
		}
		if !find {
			panic("./server/xweb/xweb.go borken,missing server.SetRouter() before server.StartWebServer()")
		}

		var buf bytes.Buffer
		if e := format.Node(&buf, fset, file); e != nil {
			panic("format ast tree to go code failed,error:" + e.Error())
		}
		if e := os.WriteFile("./server/xweb/xweb.go", buf.Bytes(), 0644); e != nil {
			panic("write formatted go code from ast tree to file ./server/xweb/xweb.go failed,error:" + e.Error())
		}
	}

	func updateSubProjectXcrpc() {
		fset := token.NewFileSet()
		file, e := parser.ParseFile(fset, "./server/xcrpc/xcrpc.go", nil, parser.ParseComments)
		if e != nil {
			panic("./server/xcrpc/xcrpc.go parse failed,error:" + e.Error())
		}
		var start *ast.FuncDecl
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "StartCrpcServer" {
				start = f
				break
			}
		}
		if start == nil || start.Body == nil || len(start.Body.List) == 0 {
			panic("./server/xcrpc/xcrpc.go broken,missing func StartCrpcServer")
		}
		registerStmt := &ast.ExprStmt{
			// api.RegisterxxxCrpcServer(r, service.Svcxxx, mids.AllMids())
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("api"),
					Sel: ast.NewIdent("Register" + string((*gensub)[0]-32) + (*gensub)[1:] + "CrpcServer"),
				},
				Args: []ast.Expr{
					ast.NewIdent("server"),
					&ast.SelectorExpr{
						X:   ast.NewIdent("service"),
						Sel: ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:]),
					},
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("mids"),
							Sel: ast.NewIdent("AllMids"),
						},
					},
				},
			},
		}
		find := false
		//insert the registerStmt before server.StartCrpcServer
		for i := len(start.Body.List) - 1; i >= 0; i-- {
			stmt := start.Body.List[i]
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok {
				continue
			}
			initAssign, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok || initAssign.Tok != token.ASSIGN {
				continue
			}
			if len(initAssign.Rhs) != 1 {
				continue
			}
			call, ok := initAssign.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if xIdent, ok := sel.X.(*ast.Ident); !ok || xIdent.Name != "server" {
				continue
			}
			if sel.Sel.Name != "StartCrpcServer" {
				continue
			}
			find = true
			newlist := make([]ast.Stmt, 0, len(start.Body.List)+1)
			newlist = append(newlist, start.Body.List[:i]...)
			newlist = append(newlist, registerStmt)
			newlist = append(newlist, start.Body.List[i:]...)
			start.Body.List = newlist
			break
		}
		if !find {
			panic("./server/xcrpc/xcrpc.go borken,missing server.StartCrpcServer()")
		}

		var buf bytes.Buffer
		if e := format.Node(&buf, fset, file); e != nil {
			panic("format ast tree to go code failed,error:" + e.Error())
		}
		if e := os.WriteFile("./server/xcrpc/xcrpc.go", buf.Bytes(), 0644); e != nil {
			panic("write formatted go code from ast tree to file ./server/xcrpc/xcrpc.go failed,error:" + e.Error())
		}
	}

	func updateSubProjectXgrpc() {
		fset := token.NewFileSet()
		file, e := parser.ParseFile(fset, "./server/xgrpc/xgrpc.go", nil, parser.ParseComments)
		if e != nil {
			panic("./server/xgrpc/xgrpc.go parse failed,error:" + e.Error())
		}
		var start *ast.FuncDecl
		for _, decl := range file.Decls {
			if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "StartCGrpcServer" {
				start = f
				break
			}
		}
		if start == nil || start.Body == nil || len(start.Body.List) == 0 {
			panic("./server/xgrpc/xgrpc.go broken,missing func StartCGrpcServer")
		}
		registerStmt := &ast.ExprStmt{
			// api.RegisterxxxCGrpcServer(r, service.Svcxxx, mids.AllMids())
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("api"),
					Sel: ast.NewIdent("Register" + string((*gensub)[0]-32) + (*gensub)[1:] + "CGrpcServer"),
				},
				Args: []ast.Expr{
					ast.NewIdent("server"),
					&ast.SelectorExpr{
						X:   ast.NewIdent("service"),
						Sel: ast.NewIdent("Svc" + string((*gensub)[0]-32) + (*gensub)[1:]),
					},
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("mids"),
							Sel: ast.NewIdent("AllMids"),
						},
					},
				},
			},
		}
		find := false
		//insert the registerStmt before server.StartCGrpcServer
		for i := len(start.Body.List) - 1; i >= 0; i-- {
			stmt := start.Body.List[i]
			ifStmt, ok := stmt.(*ast.IfStmt)
			if !ok {
				continue
			}
			initAssign, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok || initAssign.Tok != token.ASSIGN {
				continue
			}
			if len(initAssign.Rhs) != 1 {
				continue
			}
			call, ok := initAssign.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			if xIdent, ok := sel.X.(*ast.Ident); !ok || xIdent.Name != "server" {
				continue
			}
			if sel.Sel.Name != "StartCGrpcServer" {
				continue
			}
			find = true
			newlist := make([]ast.Stmt, 0, len(start.Body.List)+1)
			newlist = append(newlist, start.Body.List[:i]...)
			newlist = append(newlist, registerStmt)
			newlist = append(newlist, start.Body.List[i:]...)
			start.Body.List = newlist
			break
		}
		if !find {
			panic("./server/xgrpc/xgrpc.go borken,missing server.StartCGrpcServer()")
		}

		var buf bytes.Buffer
		if e := format.Node(&buf, fset, file); e != nil {
			panic("format ast tree to go code failed,error:" + e.Error())
		}
		if e := os.WriteFile("./server/xgrpc/xgrpc.go", buf.Bytes(), 0644); e != nil {
			panic("write formatted go code from ast tree to file ./server/xgrpc/xgrpc.go failed,error:" + e.Error())
		}
	}
*/
func createKubernetes() {
	var input string
	for len(input) == 0 {
		fmt.Printf("this will delete the old ./Dockerfile and ./deployment.yaml(if exist),then create the new one,continue? [y/n]: ")
		if _, e := fmt.Scanln(&input); e != nil {
			if e.Error() == "unexpected newline" {
				input = ""
				continue
			}
			panic(e)
		}
		input = strings.TrimSpace(input)
		if len(input) == 0 || (input[0] != 'y' && input[0] != 'n') {
			input = ""
			continue
		}
	}
	if input[0] == 'n' {
		fmt.Println("abort")
		return
	}
	var needservice bool
	var needgateway bool
	input = ""
	for len(input) == 0 {
		fmt.Printf("need kubernetes service? [y/n]: ")
		if _, e := fmt.Scanln(&input); e != nil {
			if e.Error() == "unexpected newline" {
				input = ""
				continue
			}
			panic(e)
		}
		input = strings.TrimSpace(input)
		if len(input) == 0 || ((input)[0] != 'y' && (input)[0] != 'n') {
			input = ""
			continue
		}
	}
	if input[0] == 'y' {
		needservice = true
	}
	if needservice {
		input = ""
		for len(input) == 0 {
			fmt.Printf("need kubernetes gateway? [y/n]: ")
			if _, e := fmt.Scanln(&input); e != nil {
				if e.Error() == "unexpected newline" {
					input = ""
					continue
				}
				panic(e)
			}
			input = strings.TrimSpace(input)
			if len(input) == 0 || ((input)[0] != 'y' && (input)[0] != 'n') {
				input = ""
				continue
			}
		}
		if input[0] == 'y' {
			needgateway = true
		}
	}
	if e := os.Remove("./Dockerfile"); e != nil {
		if !os.IsNotExist(e) {
			panic("delete old ./Dockerfile error: " + e.Error())
		}
	}
	if e := os.Remove("./deployment.yaml"); e != nil {
		if !os.IsNotExist(e) {
			panic("delete old ./deployment.yaml error: " + e.Error())
		}
	}
	fmt.Println("start create kubernetes config.")
	deploy.CreatePathAndFile(*appname, needservice, needgateway)
	fmt.Println("kubernetes config create success!")
}
func createHtml() {
	var input string
	for len(input) == 0 {
		fmt.Printf("this will delete the old ./html dir(if exist),then create the new one,continue? [y/n]: ")
		if _, e := fmt.Scanln(&input); e != nil {
			if e.Error() == "unexpected newline" {
				input = ""
				continue
			}
			panic(e)
		}
		input = strings.TrimSpace(input)
		if len(input) == 0 || (input[0] != 'y' && input[0] != 'n') {
			input = ""
			continue
		}
	}
	if input[0] == 'n' {
		fmt.Println("abort")
		return
	}
	if e := os.RemoveAll("./html"); e != nil {
		if !os.IsNotExist(e) {
			panic("delete old ./html dir error: " + e.Error())
		}
	}
	fmt.Println("start create html.")
	html.CreatePathAndFile(*appname)
	fmt.Println("html create success!")
	fmt.Println()
	fmt.Println("cd html")
	fmt.Println("npm install")
	fmt.Println("npm run dev")
	fmt.Println("npm run build")
}
