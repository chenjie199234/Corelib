package git

import (
	"fmt"
	"os"
	"text/template"
)

const text = `*
!.gitignore
!/api/
!/api/*
!/api/**/
!/api/**/*
!/config/
!/config/*
!/config/**/
!/config/**/*
!/dao/
!/dao/*
!/dao/**/
!/dao/**/*
!/model/
!/model/*
!/model/**/
!/model/**/*
!/server/
!/server/*
!/server/**/
!/server/**/*
!/service/
!/service/*
!/service/**/
!/service/**/*
!AppConfig.json
!cmd.sh
!cmd.bat
!probe.sh
!deployment.yaml
!Dockerfile
!go.mod
!main.go
!README.md
!SourceConfig.json`

const path = "./"
const name = ".gitignore"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("ignore").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+name, e))
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
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+name, e))
	}
}
