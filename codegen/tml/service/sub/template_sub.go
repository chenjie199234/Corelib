package sub

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package {{.Sname}}

import (
	"context"

	"{{.PackageName}}/config"
	"{{.PackageName}}/api"
	{{.Sname}}dao "{{.PackageName}}/dao/{{.Sname}}"
	"{{.PackageName}}/ecode"

	//"github.com/chenjie199234/Corelib/cgrpc"
	//"github.com/chenjie199234/Corelib/crpc"
	//"github.com/chenjie199234/Corelib/log"
	//"github.com/chenjie199234/Corelib/web"
	"github.com/chenjie199234/Corelib/util/graceful"
)

// Service subservice for {{.Sname}} business
type Service struct {
	stop *graceful.Graceful

	{{.Sname}}Dao *{{.Sname}}dao.Dao
}

// Start -
func Start() *Service {
	return &Service{
		stop: graceful.New(),

		{{.Sname}}Dao: {{.Sname}}dao.NewDao(config.GetSql("{{.Sname}}_sql"), config.GetRedis("{{.Sname}}_redis"), config.GetMongo("{{.Sname}}_mongo")),
	}
}

// Stop -
func (s *Service) Stop() {
	s.stop.Close(nil, nil)
}`

const path = "./service/"

var tml *template.Template
var file *os.File

type data struct {
	PackageName string
	ProjectName string
	Sname       string
}

func init() {
	var e error
	tml, e = template.New("sub").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile(sname string) {
	var e error
	if e = os.MkdirAll(path+sname+"/", 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path+sname, e))
	}
	file, e = os.OpenFile(path+sname+"/service.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+"/service.go", e))
	}
}
func Execute(PackageName, ProjectName, Sname string) {
	if e := tml.Execute(file, &data{PackageName: PackageName, Sname: Sname}); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+Sname+"/"+Sname+".go", e))
	}
}
