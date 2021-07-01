package service

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package service

import (
	"{{.}}/dao"
	"{{.}}/service/status"
)

//SvcStatus one specify sub service
var SvcStatus *status.Service

//StartService start the whole service
func StartService() error {
	if e := dao.NewApi(); e != nil {
		return e
	}
	//start sub service
	SvcStatus = status.Start()
	return nil
}

//StopService stop the whole service
func StopService() {
	//stop sub service
	SvcStatus.Stop()
}`

const path = "./service/"
const name = "service.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("service").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+name, e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
