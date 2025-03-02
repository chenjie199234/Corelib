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

const Name = "{{.}}"

var Group = os.Getenv("GROUP")
var Project = os.Getenv("PROJECT")

func init() {
	if Group == "" || Group == "<GROUP>" {
	  panic("missing env:GROUP")
	}
	if Project == "" || Project == "<PROJECT>" {
	  panic("missing env:PROJECT")
	}
	if e := name.SetSelfFullName(Project, Group, Name); e != nil {
	  panic(e)
	}
}`

func CreatePathAndFile(appname string) {
	if e := os.MkdirAll("./model/", 0755); e != nil {
		panic("mkdir ./model/ error: " + e.Error())
	}
	modeltemplate, e := template.New("./model/model.go").Parse(txt)
	if e != nil {
		panic("parse ./model/model.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./model/model.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./model/model.go error: " + e.Error())
	}
	if e := modeltemplate.Execute(file, appname); e != nil {
		panic("write ./model/model.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./model/model.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./model/model.go error: " + e.Error())
	}
}
