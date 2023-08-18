package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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
	"github.com/chenjie199234/Corelib/codegen/tml/npm"
	"github.com/chenjie199234/Corelib/codegen/tml/readme"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xcrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xgrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xweb"
	"github.com/chenjie199234/Corelib/codegen/tml/service"
	servicestatus "github.com/chenjie199234/Corelib/codegen/tml/service/status"
	"github.com/chenjie199234/Corelib/codegen/tml/service/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/util"
	cname "github.com/chenjie199234/Corelib/util/name"
)

var projectname = flag.String("n", "", "project's name\ncharacter:[a-z][0-9]\nfirst character must in [a-z]")
var packagename = flag.String("p", "", "project's package name\npackage name must end with project's name\nif this is empty the project's name will be used as the package's name\nthis is useful when your project will be uploaded to github or gitlab\ne.g. github.com/path_to_the_repo/project_name")

var gensub = flag.String("sub", "", "create subservice in this project\ncharacter:[a-z][0-9]\nfirst character must in [a-z]\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genkube = flag.Bool("kube", false, "create project's kubernetes config file\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var genhtml = flag.Bool("html", false, "create project's html template\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")

func main() {
	flag.Parse()
	//pre check
	if e := cname.SingleCheck(*projectname, false); e != nil {
		panic(e)
	}
	if *packagename == "" {
		packagename = projectname
	}
	if *packagename != *projectname && !strings.HasSuffix(*packagename, "/"+*projectname) {
		panic("package's name must end with project's name,e.g. github.com/path_to_the_repo/project_name")
	}
	if len(*gensub) == 0 && !*genhtml && !*genkube {
		//create the base project
		createBaseProject()
		return
	}
	checkBaseProject()
	if *genkube {
		createKubernetes()
	}
	if len(*gensub) != 0 {
		//create sub service in this project
		createSubProject()
	}
	if *genhtml {
		//create project's html template
		createHtml()
	}
}

func createBaseProject() {
	finfo, e := os.Stat("./" + *projectname)
	if e != nil {
		if !os.IsNotExist(e) {
			panic("get ./" + *projectname + " info error: " + e.Error())
		}
		if e := os.MkdirAll("./"+*projectname, 0755); e != nil {
			panic("mkdir ./" + *projectname + " error: " + e.Error())
		}
	} else if !finfo.IsDir() {
		panic("./" + *projectname + " exist and it is not a dir")
	} else if files, e := os.ReadDir("./" + *projectname); e != nil {
		panic("./" + *projectname + " check dir empty error: " + e.Error())
	} else if len(files) > 0 {
		panic("./" + *projectname + " exist and it is not an empty dir")
	}
	if e = os.Chdir("./" + *projectname); e != nil {
		panic("cd ./" + *projectname + " error: " + e.Error())
	}
	//pre check success
	fmt.Println("start create base project.")
	api.CreatePathAndFile()

	statusapi.CreatePathAndFile(*packagename, *projectname)

	ecode.CreatePathAndFile()

	config.CreatePathAndFile(*packagename)

	configfile.CreatePathAndFile(*projectname)

	dao.CreatePathAndFile(*packagename)

	subdao.CreatePathAndFile("status")

	mainfile.CreatePathAndFile(*packagename)

	gomod.CreatePathAndFile(*packagename)

	model.CreatePathAndFile(*packagename, *projectname)
	submodel.CreatePathAndFile("status")

	util.CreatePathAndFile()

	xcrpc.CreatePathAndFile(*packagename)

	xgrpc.CreatePathAndFile(*packagename)

	xweb.CreatePathAndFile(*packagename)

	service.CreatePathAndFile(*packagename)

	servicestatus.CreatePathAndFile(*packagename)

	cmd.CreatePathAndFile(*packagename, *projectname)

	readme.CreatePathAndFile(*projectname)

	git.CreatePathAndFile()

	npm.CreatePathAndFile()
	fmt.Println("base project create success!")
}
func checkBaseProject() {
	f, e := os.Open("./model/model.go")
	if e != nil {
		panic("open ./model/model.go error: " + e.Error())
	}
	bio := bufio.NewReader(f)
	var tmppackage, tmpproject string
	for {
		line, _, e := bio.ReadLine()
		if e != nil {
			if e == io.EOF {
				break
			}
			panic("read ./model/model.go error: " + e.Error())
		}
		str := strings.TrimSpace(string(line))
		if strings.HasPrefix(str, "const Name = ") {
			tmpproject = str[13:]
			if len(tmpproject) <= 2 || tmpproject[0] != '"' || tmpproject[len(tmpproject)-1] != '"' {
				panic("./model/model.go broken!")
			}
			tmpproject = tmpproject[1 : len(tmpproject)-1]
		}
		if strings.HasPrefix(str, "const pkg = ") {
			tmppackage = str[12:]
			if len(tmppackage) <= 2 || tmppackage[0] != '"' || tmppackage[len(tmppackage)-1] != '"' {
				panic("./model/model.go broken!")
			}
			tmppackage = tmppackage[1 : len(tmppackage)-1]
		}
		if tmppackage != "" && tmpproject != "" {
			break
		}
	}
	if tmppackage == "" || tmpproject == "" {
		panic("./model/model.go broken!")
	}
	if tmppackage != *packagename {
		panic("package name conflict,this is not the required project")
	}
	if tmpproject != *projectname {
		panic("project name conflict,this is not the required project")
	}
}

func createSubProject() {
	//create sub project
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
	subapi.CreatePathAndFile(*packagename, *projectname, *gensub)
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
	deploy.CreatePathAndFile(*projectname, needservice, needingress)
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
	html.CreatePathAndFile(*projectname)
	fmt.Println("html create success!")
	fmt.Println()
	fmt.Println("cd html")
	fmt.Println("npm install")
	fmt.Println("npm run dev")
	fmt.Println("npm run build")
}
