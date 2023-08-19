package model

import (
	"os"
	"text/template"
)

const txt = `package model

import (
	"os"

	"github.com/chenjie199234/Corelib/util/name"
)

// Warning!!!!!!!!!!!
// This file is readonly!
// Don't modify this file!

const pkg = "{{.PackageName}}"
const Name = "{{.AppName}}"

var Group = os.Getenv("GROUP")
var Project = os.Getenv("PROJECT")

func init() {
	if Group == "" || Group == "<GROUP>" {
		panic("missing GROUP env")
	}
	if name.SingleCheck(Group, false) != nil {
		panic("env GROUP format wrong")
	}
	if Project == "" || Project == "<PROJECT>" {
		panic("missing PROJECT env")
	}
	if name.SingleCheck(Project, false) != nil {
		panic("env PROJECT format wrong")
	}
}`

type data struct {
	PackageName string
	AppName     string
}

func CreatePathAndFile(packagename, appname string) {
	if e := os.MkdirAll("./model/", 0755); e != nil {
		panic("mkdir ./model/ error: " + e.Error())
	}
	tmp := &data{
		PackageName: packagename,
		AppName:     appname,
	}
	modeltemplate, e := template.New("./model/model.go").Parse(txt)
	if e != nil {
		panic("parse ./model/model.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./model/model.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./model/model.go error: " + e.Error())
	}
	if e := modeltemplate.Execute(file, tmp); e != nil {
		panic("write ./model/model.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./model/model.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./model/model.go error: " + e.Error())
	}
}
