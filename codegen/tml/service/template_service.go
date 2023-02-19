package service

import (
	"os"
	"text/template"
)

const txt = `package service

import (
	"{{.}}/dao"
	"{{.}}/service/status"
)

// SvcStatus one specify sub service
var SvcStatus *status.Service

// StartService start the whole service
func StartService() error {
	if e := dao.NewApi(); e != nil {
		return e
	}
	//start sub service
	SvcStatus = status.Start()
	return nil
}

// StopService stop the whole service
func StopService() {
	//stop sub service
	SvcStatus.Stop()
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
