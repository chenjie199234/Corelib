package gomod

import (
	"fmt"
	"os"
	"text/template"

	"github.com/chenjie199234/Corelib/internal/version"
)

const txt = `module {{.}}

go 1.26.2

require (
	github.com/chenjie199234/admin main
	github.com/chenjie199234/Corelib %s
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-sql-driver/mysql v1.9.3
	github.com/redis/go-redis/v9 v9.21.0
	go.mongodb.org/mongo-driver/v2 v2.8.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af
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
