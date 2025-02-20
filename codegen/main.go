package main

import (
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
	cname "github.com/chenjie199234/Corelib/util/name"
)

var appname = flag.String("n", "", "app name\ncharacter:[a-z][0-9]\nfirst character must in [a-z]")
var packagename = flag.String("p", "", "package name\nmust be app name or end with app name\nif this is empty the app name will be used as the package name\nthis is useful when your project will be uploaded to github or gitlab\ne.g. github.com/path_to_the_repo/app_name")

var gensub = flag.String("sub", "", "create subservice in this app\ncharacter:[a-z][0-9]\nfirst character must in [a-z]\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genkube = flag.Bool("kube", false, "create project's kubernetes config file\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genhtml = flag.Bool("html", false, "create project's html template\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")

func init() {
	flag.Parse()
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
func main() {
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
	_, e := os.Stat("./api/" + *gensub + ".proto")
	if e == nil {
		panic("./api/" + *gensub + ".proto already exist")
	}
	if !os.IsNotExist(e) {
		panic("./api/" + *gensub + ".proto check file exist error: " + e.Error())
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
	fmt.Println("sub service create success!")
}
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
	var needingress bool
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
			fmt.Printf("need kubernetes ingress? [y/n]: ")
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
			needingress = true
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
	deploy.CreatePathAndFile(*appname, needservice, needingress)
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
