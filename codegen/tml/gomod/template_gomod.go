package gomod

import (
	"fmt"
	"os"
	"text/template"

	"github.com/chenjie199234/Corelib/internal/version"
)

const text = `module {{.}}

go 1.18

require (
	github.com/chenjie199234/admin main
	github.com/chenjie199234/Corelib %s
	github.com/fsnotify/fsnotify v1.6.0
	github.com/go-sql-driver/mysql v1.7.0
	github.com/segmentio/kafka-go v0.4.38
	go.mongodb.org/mongo-driver v1.11.1
	google.golang.org/protobuf v1.28.1
)`

const path = "./"
const name = "go.mod"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("gomod").Parse(fmt.Sprintf(text, version.String()))
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
func Execute(PackageName string) {
	if e := tml.Execute(file, PackageName); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
