package service

import (
	"os"
	"text/template"
)

const txt = `package service

import (
	"{{.}}/dao"
	"{{.}}/service/raw"
	"{{.}}/service/status"
)

var SvcStatus *status.Service

var SvcRaw *raw.Service

func StartService() error {
	var e error
	if e = dao.NewApi(); e != nil {
		return e
	}
	if SvcRaw, e = raw.Start(); e != nil {
		return e
	}
	if SvcStatus, e = status.Start(); e != nil {
		return e
	}
	return nil
}

func StopService() {
	SvcStatus.Stop()
	SvcRaw.Stop()
}`

func CreatePathAndFile(packagename string) {
	if e := os.MkdirAll("./service/", 0755); e != nil {
		panic("mkdir ./service/ error: " + e.Error())
	}
	servicetemplate, e := template.New("./service/service.go").Parse(txt)
	if e != nil {
		panic("parse ./service/service.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./service/service.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./service/service.go error: " + e.Error())
	}
	if e := servicetemplate.Execute(file, packagename); e != nil {
		panic("write ./service/service.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./service/service.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./service/service.go error: " + e.Error())
	}
}
