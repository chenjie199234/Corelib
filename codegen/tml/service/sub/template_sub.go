package sub

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package {{.Sname}}

import (
	"context"

	"{{.Pname}}/config"
	"{{.Pname}}/api"
	{{.Sname}}dao "{{.Pname}}/dao/{{.Sname}}"
	"{{.Pname}}/ecode"

	//"github.com/chenjie199234/Corelib/log"
	//"github.com/chenjie199234/Corelib/grpc"
	//"github.com/chenjie199234/Corelib/crpc"
	//"github.com/chenjie199234/Corelib/web"
)

//Service subservice for {{.Sname}} business
type Service struct {
	{{.Sname}}Dao *{{.Sname}}dao.Dao
}

//Start -
func Start() *Service {
	return &Service{
		{{.Sname}}Dao: {{.Sname}}dao.NewDao(config.GetSql("{{.Sname}}_sql"), config.GetRedis("{{.Sname}}_redis"), config.GetMongo("{{.Sname}}_mongo")),
	}
}

//Stop -
func (s *Service) Stop() {

}`

const path = "./service/"

var tml *template.Template
var file *os.File

type data struct {
	Pname string
	Sname string
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
	file, e = os.OpenFile(path+sname+"/"+sname+".go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+"/"+sname+".go", e))
	}
}
func Execute(pname, sname string) {
	if e := tml.Execute(file, &data{Pname: pname, Sname: sname}); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+sname+"/"+sname+".go", e))
	}
}
