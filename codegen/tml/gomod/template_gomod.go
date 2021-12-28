package gomod

import (
	"fmt"
	"os"
	"text/template"
)

const text = `module {{.}}

go 1.17

require (
	github.com/chenjie199234/Config v0.0.22
	github.com/chenjie199234/Corelib v0.0.36
	github.com/fsnotify/fsnotify v1.5.1
	github.com/go-sql-driver/mysql v1.6.0
	github.com/segmentio/kafka-go v0.4.25
	go.mongodb.org/mongo-driver v1.8.1
	google.golang.org/protobuf v1.27.1
)`

const path = "./"
const name = "go.mod"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("gomod").Parse(text)
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
