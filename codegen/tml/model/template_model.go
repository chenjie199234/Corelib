package model

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package model

import "os"

// Warning!!!!!!!!!!!
// This file is readonly!
// Don't modify this file!

const pkg = "{{.PackageName}}"
const Name = "{{.ProjectName}}"

var Group = os.Getenv("GROUP")

func init() {
	if Group == "" || Group == "<GROUP>" {
		panic("missing GROUP env")
	}
}`

const path = "./model/"
const name = "model.go"

type data struct {
	PackageName string
	ProjectName string
}

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("model").Parse(text)
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
func Execute(PackageName, ProjectName string) {
	if e := tml.Execute(file, &data{PackageName: PackageName, ProjectName: ProjectName}); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
