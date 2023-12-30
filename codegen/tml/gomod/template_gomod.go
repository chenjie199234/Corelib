package gomod

import (
	"fmt"
	"os"
	"text/template"

	"github.com/chenjie199234/Corelib/internal/version"
)

const txt = `module {{.}}

go 1.21

require (
	github.com/chenjie199234/admin main
	github.com/chenjie199234/Corelib %s
	github.com/fsnotify/fsnotify v1.7.0
	github.com/go-sql-driver/mysql v1.7.1
	github.com/redis/go-redis/v9 v9.3.1
	go.mongodb.org/mongo-driver v1.13.1
	google.golang.org/grpc v1.60.0
	google.golang.org/protobuf v1.32.0
)`

func CreatePathAndFile(packagename string) {
	gomodtemplate, e := template.New("./go.mod").Parse(fmt.Sprintf(txt, version.String()))
	if e != nil {
		panic("parse ./go.mod template error: " + e.Error())
	}
	file, e := os.OpenFile("./go.mod", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./go.mod error: " + e.Error())
	}
	if e := gomodtemplate.Execute(file, packagename); e != nil {
		panic("write ./go.mod error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./go.mod error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./go.mod error: " + e.Error())
	}
}
