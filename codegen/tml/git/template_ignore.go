package git

import (
	"os"
)

const txt = `*
!.gitignore
!/api/
!/api/*
!/api/**/
!/api/**/*
!/ecode/
!/ecode/*
!/ecode/**/
!/ecode/**/*
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
!/util/
!/util/*
!/util/**/
!/util/**/*
!/html/
!/html/*
!/html/**/
!/html/**/*
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

func CreatePathAndFile() {
	file, e := os.OpenFile("./.gitignore", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./.gitignore error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./.gitignore error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./.gitignore error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./.gitignore error: " + e.Error())
	}
}
