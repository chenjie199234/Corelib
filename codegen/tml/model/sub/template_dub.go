package sub

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package model`
const path = "./model/"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("model").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile(sname string) {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+sname+".go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+".go", e))
	}
}
func Execute(sname string) {
	if e := tml.Execute(file, nil); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+sname+".go", e))
	}
}
