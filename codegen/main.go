package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	clientapi "github.com/chenjie199234/Corelib/codegen/tml/api/client"
	statusapi "github.com/chenjie199234/Corelib/codegen/tml/api/status"
	subapi "github.com/chenjie199234/Corelib/codegen/tml/api/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/cmd"
	"github.com/chenjie199234/Corelib/codegen/tml/config"
	"github.com/chenjie199234/Corelib/codegen/tml/configfile"
	"github.com/chenjie199234/Corelib/codegen/tml/dao"
	subdao "github.com/chenjie199234/Corelib/codegen/tml/dao/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/ecode"
	"github.com/chenjie199234/Corelib/codegen/tml/git"
	"github.com/chenjie199234/Corelib/codegen/tml/gomod"
	"github.com/chenjie199234/Corelib/codegen/tml/kubernetes"
	"github.com/chenjie199234/Corelib/codegen/tml/mainfile"
	"github.com/chenjie199234/Corelib/codegen/tml/model"
	submodel "github.com/chenjie199234/Corelib/codegen/tml/model/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/readme"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xcrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xgrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xweb"
	"github.com/chenjie199234/Corelib/codegen/tml/service"
	servicestatus "github.com/chenjie199234/Corelib/codegen/tml/service/status"
	subservice "github.com/chenjie199234/Corelib/codegen/tml/service/sub"
	cname "github.com/chenjie199234/Corelib/util/name"
)

var name = flag.String("n", "", "project's name\ncharacter:[a-z][A-Z][0-9][_]\nfirst character must in [a-z][A-Z]")
var packagename = flag.String("p", "", "project's package name\npackage name must end with project's name\nif this is empty the project's name will be used as the package name\nthis is useful when your project will be uploaded to github or gitlab\ne.g. github.com/path_to_the_repo/project_name")
var dir = flag.String("d", "", "project's create dir")
var sub = flag.String("s", "", "create subservice's name in project\ncharacter:[a-z][A-Z][0-9][_]\nfirst character must in [a-z][A-Z]\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var kub = flag.Bool("k", false, "update exist project's kubernetes config file\ndon't use this direct by codegen,use the cmd.sh/cmd.bat in your project instead")
var needkubernetes bool
var needkubernetesservice bool
var needkubernetesingress bool

func main() {
	flag.Parse()
	//pre check
	if e := cname.NameCheck(*name); e != nil {
		panic(e)
	}
	if *packagename == "" {
		packagename = name
	} else if *packagename != *name && !strings.HasSuffix(*packagename, "/"+*name) {
		panic("package name must end with project name,e.g. github.com/path_to_the_repo/project_name")
	}
	if len(*sub) != 0 {
		if e := cname.NameCheck(*sub); e != nil {
			panic(e)
		}
		checkBaseProjectName()
		createSubProject()
		return
	}
	if *kub {
		checkBaseProjectName()
		createkubernetes()
	} else {
		createBaseProject()
	}
}
func checkBaseProjectName() {
	data, e := os.ReadFile("./api/client.go")
	if e != nil {
		panic("please change dir to project's root dir,then run this manually or run the cmd script")
	}
	bio := bufio.NewReader(bytes.NewBuffer(data))
	var tmppackage, tmpproject string
	for {
		line, _, e := bio.ReadLine()
		if e != nil {
			if e != io.EOF {
				panic("read api/client.go error:" + e.Error())
			}
			break
		}
		str := strings.TrimSpace(string(line))
		if strings.HasPrefix(str, "const Name = ") {
			tmpproject = str[13:]
			if len(tmpproject) <= 2 || tmpproject[0] != '"' || tmpproject[len(tmpproject)-1] != '"' {
				panic("api/client.go broken!")
			}
			tmpproject = tmpproject[1 : len(tmpproject)-1]
		}
		if strings.HasPrefix(str, "const pkg = ") {
			tmppackage = str[12:]
			if len(tmppackage) <= 2 || tmppackage[0] != '"' || tmppackage[len(tmppackage)-1] != '"' {
				panic("api/client.go broken!")
			}
			tmppackage = tmppackage[1 : len(tmppackage)-1]
		}
		if tmppackage != "" && tmpproject != "" {
			break
		}
	}
	if tmppackage == "" || tmpproject == "" {
		panic("api/client.go broken!")
	}
	if tmppackage != *packagename || tmpproject != *name {
		panic("please change dir to project's root dir first")
	}
}
func createSubProject() {
	//create sub project
	_, e := os.Stat("./api/" + *sub + ".proto")
	if e == nil {
		panic(fmt.Sprintf("can't create sub service,'./api/%s.proto' file already exist", *sub))
	}
	if !os.IsNotExist(e) {
		panic(fmt.Sprintf("can't create sub service,get './api/%s.proto' file info error:%s", *sub, e))
	}
	_, e = os.Stat("./service/" + *sub)
	if e == nil {
		panic(fmt.Sprintf("can't create sub service,'./service/%s' dir already exist", *sub))
	}
	if !os.IsNotExist(e) {
		panic(fmt.Sprintf("can't create sub service,get './service/%s' dir info error:%s", *sub, e))
	}
	_, e = os.Stat("./dao/" + *sub)
	if e == nil {
		panic(fmt.Sprintf("can't create sub service,'./dao/%s' dir already exist", *sub))
	}
	if !os.IsNotExist(e) {
		panic(fmt.Sprintf("can't create sub service,get './dao/%s' dir info error:%s", *sub, e))
	}
	//sub api
	subapi.CreatePathAndFile(*sub)
	subapi.Execute(*packagename, *name, *sub)
	//sub dao
	subdao.CreatePathAndFile(*sub)
	subdao.Execute(*sub)
	//sub service
	subservice.CreatePathAndFile(*sub)
	subservice.Execute(*packagename, *name, *sub)
	//sub model
	submodel.CreatePathAndFile(*sub)
	submodel.Execute(*sub)
}
func createBaseProject() {
	//create project
	if *dir == "" {
		*dir = "./"
	}
	finfo, e := os.Stat(*dir)
	if e != nil {
		if !os.IsNotExist(e) {
			panic(fmt.Sprintf("get project's create dir:%s info error:%s", *dir, e))
		}
		if e = os.MkdirAll(*dir, 0755); e != nil {
			panic(fmt.Sprintf("project's create dir:%s not exist and create error:%s", *dir, e))
		}
	} else if !finfo.IsDir() {
		panic(fmt.Sprintf("project's create dir:%s exist and is not a dir", *dir))
	}
	if e = os.Chdir(*dir); e != nil {
		panic(fmt.Sprintf("enter project's create dir:%s error:%s", *dir, e))
	}
	finfo, e = os.Stat("./" + *name)
	if e != nil {
		if !os.IsNotExist(e) {
			panic(fmt.Sprintf("get project's dir:%s in create dir:%s info error:%s", "./"+*name, *dir, e))
		}
		if e = os.MkdirAll("./"+*name, 0755); e != nil {
			panic(fmt.Sprintf("project's dir:%s in project's create dir:%s not exist and create error:%s", "./"+*name, *dir, e))
		}
	} else if !finfo.IsDir() {
		panic(fmt.Sprintf("project's dir:%s in project's create dir:%s is not a dir", "./"+*name, *dir))
	} else {
		files, e := os.ReadDir("./" + *name)
		if e != nil {
			panic(fmt.Sprintf("read project's dir:%s in project's create dir:%s error:%s", "./"+*name, *dir, e))
		}
		if len(files) > 0 {
			panic(fmt.Sprintf("project's dir:%s in project's create dir:%s is not empty", "./"+*name, *dir))
		}
	}
	if e = os.Chdir("./" + *name); e != nil {
		panic(fmt.Sprintf("enter project's dir:%s in project's create dir:%s error:%s", "./"+*name, *dir, e))
	}
	//pre check success
	fmt.Println("start create base project.")

	statusapi.CreatePathAndFile()
	statusapi.Execute(*packagename, *name)

	ecode.CreatePathAndFile()
	ecode.Execute()

	config.CreatePathAndFile()
	config.Execute(*packagename)

	configfile.CreatePathAndFile()
	configfile.Execute(*name)

	dao.CreatePathAndFile()
	dao.Execute(*packagename)

	subdao.CreatePathAndFile("status")
	subdao.Execute("status")

	mainfile.CreatePathAndFile()
	mainfile.Execute(*packagename)

	gomod.CreatePathAndFile()
	gomod.Execute(*packagename)

	model.CreatePathAndFile()
	model.Execute()

	xcrpc.CreatePathAndFile()
	xcrpc.Execute(*packagename)

	xgrpc.CreatePathAndFile()
	xgrpc.Execute(*packagename)

	xweb.CreatePathAndFile()
	xweb.Execute(*packagename)

	service.CreatePathAndFile()
	service.Execute(*packagename)

	servicestatus.CreatePathAndFile()
	servicestatus.Execute(*packagename)

	cmd.CreatePathAndFile()
	cmd.Execute(*packagename, *name)

	readme.CreatePathAndFile()
	readme.Execute(*name)

	git.CreatePathAndFile()
	git.Execute()

	clientapi.CreatePathAndFile()
	clientapi.Execute(*packagename, *name)

	fmt.Println("base project create success!")
	createkubernetes()
}
func createkubernetes() {
	var input string
	for len(input) == 0 {
		fmt.Printf("need kubernetes? [y/n]: ")
		_, e := fmt.Scanln(&input)
		if e != nil {
			panic(e)
		}
		input = strings.TrimSpace(input)
		if len(input) == 0 || ((input)[0] != 'y' && (input)[0] != 'n') {
			input = ""
			continue
		}
		if input[0] == 'y' {
			needkubernetes = true
		}
	}
	if needkubernetes {
		input = ""
		for len(input) == 0 {
			fmt.Printf("need kubernetes service? [y/n]: ")
			_, e := fmt.Scanln(&input)
			if e != nil {
				panic(e)
			}
			input = strings.TrimSpace(input)
			if len(input) == 0 || ((input)[0] != 'y' && (input)[0] != 'n') {
				input = ""
				continue
			}
		}
		if input[0] == 'y' {
			needkubernetesservice = true
		}
	}
	if needkubernetesservice {
		input = ""
		for len(input) == 0 {
			fmt.Printf("need kubernetes ingress? [y/n]: ")
			_, e := fmt.Scanln(&input)
			if e != nil {
				panic(e)
			}
			input = strings.TrimSpace(input)
			if len(input) == 0 || ((input)[0] != 'y' && (input)[0] != 'n') {
				input = ""
				continue
			}
		}
		if input[0] == 'y' {
			needkubernetesingress = true
		}
	}
	if e := os.Remove("./Dockerfile"); e != nil {
		if !os.IsNotExist(e) {
			panic("delete old dockerfile error:" + e.Error())
		}
	}
	if e := os.Remove("./deployment.yaml"); e != nil {
		if !os.IsNotExist(e) {
			panic("delete old deployment.yaml error:" + e.Error())
		}
	}
	if needkubernetes {
		fmt.Println("start create kubernetes config.")
		kubernetes.CreatePathAndFile()
		kubernetes.Execute(*name, needkubernetesservice, needkubernetesingress)
		fmt.Println("create kubernetes config success!")
	}
}
