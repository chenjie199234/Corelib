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
	"github.com/chenjie199234/Corelib/codegen/tml/git"
	"github.com/chenjie199234/Corelib/codegen/tml/gomod"
	"github.com/chenjie199234/Corelib/codegen/tml/kubernetes"
	"github.com/chenjie199234/Corelib/codegen/tml/mainfile"
	"github.com/chenjie199234/Corelib/codegen/tml/model"
	"github.com/chenjie199234/Corelib/codegen/tml/readme"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xweb"
	"github.com/chenjie199234/Corelib/codegen/tml/service"
	servicestatus "github.com/chenjie199234/Corelib/codegen/tml/service/status"
	subservice "github.com/chenjie199234/Corelib/codegen/tml/service/sub"
	"github.com/chenjie199234/Corelib/util/common"
)

var name = flag.String("n", "", "project's name\ncharacter:[a-z][0-9]\nfirst character must in [a-z]")
var group = flag.String("g", "", "project's group\ncharacter:[a-z][0-9]\nfirst character must in [a-z]")
var dir = flag.String("d", "", "project's create dir")
var sub = flag.String("s", "", "create subservice's name in project\ncharacter:[a-z][A-Z][0-9]")
var kub = flag.Bool("k", false, "update exist project's kubernetes config file")
var needkubernetes bool
var needkubernetesservice bool
var needkubernetesheadlessservice bool
var needkubernetesingress bool
var kubernetesingresshost string

func main() {
	flag.Parse()
	//pre check
	if e := common.NameCheck(*name, false, true, false, true); e != nil {
		panic(e)
	}
	if e := common.NameCheck(*group, false, true, false, true); e != nil {
		panic(e)
	}
	if e := common.NameCheck(*group+"."+*name, true, true, false, true); e != nil {
		panic(e)
	}
	if len(*sub) == 0 {
		if *kub {
			checkBaseProjectName()
			createkubernetes()
		} else {
			createBaseProject()
		}
	} else if e := common.NameCheck(*sub, false, true, false, true); e != nil {
		panic(e)
	} else {
		checkBaseProjectName()
		createSubProject()
	}
}
func checkBaseProjectName() {
	data, e := os.ReadFile("./api/client.go")
	if e != nil {
		panic("please change dir to project's root dir,then run this manually or run the cmd script")
	}
	bio := bufio.NewReader(bytes.NewBuffer(data))
	var tempname, tempgroup string
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
			tempname = str[13:]
			if len(tempname) <= 2 || tempname[0] != '"' || tempname[len(tempname)-1] != '"' {
				panic("api/client.go broken!")
			}
			tempname = tempname[1 : len(tempname)-1]
		}
		if strings.HasPrefix(str, "const Group = ") {
			tempgroup = str[14:]
			if len(tempgroup) <= 2 || tempgroup[0] != '"' || tempgroup[len(tempgroup)-1] != '"' {
				panic("api/client.go broken!")
			}
			tempgroup = tempgroup[1 : len(tempgroup)-1]
		}
		if tempname != "" && tempgroup != "" {
			break
		}
	}
	if tempname != *name && tempgroup != *group {
		panic("please change dir to project's root dir,then run this manually or run the cmd script")
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
	subapi.Execute(*name, *sub)
	//sub dao
	subdao.CreatePathAndFile(*sub)
	subdao.Execute(*sub)
	//sub service
	subservice.CreatePathAndFile(*sub)
	subservice.Execute(*name, *sub)
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

	statusapi.CreatePathAndFile(*name)
	statusapi.Execute(*name)

	config.CreatePathAndFile()
	config.Execute(*name)

	configfile.CreatePathAndFile()
	configfile.Execute(*name)

	dao.CreatePathAndFile()
	dao.Execute(*name)

	subdao.CreatePathAndFile("status")
	subdao.Execute("status")

	mainfile.CreatePathAndFile()
	mainfile.Execute(*name)

	gomod.CreatePathAndFile()
	gomod.Execute(*name)

	model.CreatePathAndFile()
	model.Execute(*name)

	xrpc.CreatePathAndFile()
	xrpc.Execute(*name)

	xweb.CreatePathAndFile()
	xweb.Execute(*name)

	service.CreatePathAndFile()
	service.Execute(*name)

	servicestatus.CreatePathAndFile()
	servicestatus.Execute(*name)

	cmd.CreatePathAndFile()
	cmd.Execute(*name, *group)

	readme.CreatePathAndFile()
	readme.Execute(*name)

	git.CreatePathAndFile()
	git.Execute(*name)

	clientapi.CreatePathAndFile()
	clientapi.Execute(*name, *group)

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
			fmt.Printf("service headless? [y/n]:")
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
			needkubernetesheadlessservice = true
		}
	}
	if needkubernetesservice && !needkubernetesheadlessservice {
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
	if needkubernetesingress {
		input = ""
		for len(input) == 0 {
			fmt.Printf("ingress host: ")
			_, e := fmt.Scanln(&input)
			if e != nil {
				panic(e)
			}
			input = strings.TrimSpace(input)
		}
		kubernetesingresshost = input
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
		kubernetes.Execute(*name, *group, needkubernetesservice, needkubernetesheadlessservice, needkubernetesingress, kubernetesingresshost)
		fmt.Println("create kubernetes config success!")
	}
}
