package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/chenjie199234/Corelib/codegen/tml/api"
	subapi "github.com/chenjie199234/Corelib/codegen/tml/api/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/cmd"
	"github.com/chenjie199234/Corelib/codegen/tml/config"
	"github.com/chenjie199234/Corelib/codegen/tml/configfile"
	"github.com/chenjie199234/Corelib/codegen/tml/dao"
	subdao "github.com/chenjie199234/Corelib/codegen/tml/dao/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/discovery"
	"github.com/chenjie199234/Corelib/codegen/tml/git"
	"github.com/chenjie199234/Corelib/codegen/tml/gomod"
	"github.com/chenjie199234/Corelib/codegen/tml/mainfile"
	"github.com/chenjie199234/Corelib/codegen/tml/model"
	"github.com/chenjie199234/Corelib/codegen/tml/readme"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xgrpc"
	"github.com/chenjie199234/Corelib/codegen/tml/server/xhttp"
	"github.com/chenjie199234/Corelib/codegen/tml/service"
	servicestatus "github.com/chenjie199234/Corelib/codegen/tml/service/status"
	subservice "github.com/chenjie199234/Corelib/codegen/tml/service/sub"
	"github.com/chenjie199234/Corelib/codegen/tml/source"
)

var name = flag.String("n", "", "project's name\ncharacter:[a-z][A-Z][.].first character can't be[.]")
var dir = flag.String("d", "", "project's create dir")
var sub = flag.String("s", "", "subservice's name in project\ncharacter:[a-z][A-Z][.].first character can't be[.]")

func main() {
	flag.Parse()
	//pre check
	if len(*name) == 0 || (*name)[0] < 65 || ((*name)[0] > 90 && (*name)[0] < 97) || (*name)[0] > 122 {
		panic("the first character of project's name only support[a-z][A-Z]")
	}
	for _, v := range *name {
		if (v < 65 && v != 46) || (v > 90 && v < 97) || v > 122 {
			panic("project's name contains illegal character,only support[a-z][A-Z][.]")
		}
	}
	if len(*sub) == 0 {
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
			files, e := ioutil.ReadDir("./" + *name)
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
		fmt.Println("start")
		fmt.Printf("project's dir is:%s in project's create dir:%s\n", "./"+*name, *dir)

		api.CreatePathAndFile()
		api.Execute(*name)

		config.CreatePathAndFile()
		config.Execute(*name)

		source.CreatePathAndFile()
		source.Execute(*name)

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

		xgrpc.CreatePathAndFile()
		xgrpc.Execute(*name)

		xhttp.CreatePathAndFile()
		xhttp.Execute(*name)

		service.CreatePathAndFile()
		service.Execute(*name)

		servicestatus.CreatePathAndFile()
		servicestatus.Execute(*name)

		cmd.CreatePathAndFile()
		cmd.Execute(*name)

		readme.CreatePathAndFile()
		readme.Execute(*name)

		discovery.CreatePathAndFile()
		discovery.Execute(*name)

		git.CreatePathAndFile()
		git.Execute(*name)

		fmt.Println("success!")
	} else {
		//create sub project
		for _, v := range *sub {
			if v < 97 || v > 122 {
				panic("sub service's name contains illegal character,only support[a-z]")
			}
		}
		data, e := ioutil.ReadFile("./cmd.sh")
		if e != nil {
			panic("can't create sub service,please change dir to project's root dir,then run this manually or run the cmd script")
		}
		index := bytes.Index(data, []byte("CurrentProject:"))
		if index == -1 {
			panic("can't create sub service,please change dir to project's root dir,then run this manually or run the cmd script")
		}
		pname := make([]byte, 0)
		index += 15
		for {
			if data[index] != '\r' && data[index] != '\n' {
				pname = append(pname, data[index])
			} else {
				break
			}
			index++
		}
		pname = bytes.TrimSpace(pname)
		pname = bytes.TrimSuffix(pname, []byte{'"'})
		if string(pname) != *name {
			panic("can't create sub service,please change dir to project's root dir,then run this manually or run the cmd script")
		}
		_, e = os.Stat("./api/" + *sub + ".proto")
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
}
