package sub

import (
	"os"
	"text/template"
)

const txt = `package {{.Sname}}

import (
	"context"

	"{{.PackageName}}/api"
	"{{.PackageName}}/config"
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

type data struct {
	PackageName string
	Sname       string
}

func CreatePathAndFile(packagename, sname string) {
	tmp := &data{
		PackageName: packagename,
		Sname:       sname,
	}
	if e := os.MkdirAll("./service/"+sname+"/", 0755); e != nil {
		panic("mkdir ./service/" + sname + "/ error: " + e.Error())
	}
	servicetemplate, e := template.New("./service/" + sname + "/service.go").Parse(txt)
	if e != nil {
		panic("parse ./service/" + sname + "/service.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./service/"+sname+"/service.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./service/" + sname + "/service.go error: " + e.Error())
	}
	if e := servicetemplate.Execute(file, tmp); e != nil {
		panic("write ./service/" + sname + "/service.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./service/" + sname + "/service.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./service/" + sname + "/service.go error: " + e.Error())
	}
}
