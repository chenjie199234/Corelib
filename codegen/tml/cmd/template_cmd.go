package cmd

import (
	"os"
	"strings"
	"text/template"
)

const txtbash = `#!/bin/bash
#      Warning!!!!!!!!!!!This file is readonly!Don't modify this file!

cd $(dirname $0)

help() {
	echo "cmd.sh — every thing you need"
	echo "         please install git"
	echo "         please install golang(1.21+)"
	echo "         please install protoc           (github.com/protocolbuffers/protobuf)"
	echo "         please install protoc-gen-go    (github.com/protocolbuffers/protobuf-go)"
	echo "         please install codegen          (github.com/chenjie199234/Corelib)"
	echo ""
	echo "Usage:"
	echo "   ./cmd.sh <option>"
	echo ""
	echo "Options:"
	echo "   pb                        Generate the proto in this program."
	echo "   sub <sub service name>    Create a new sub service."
	echo "   kube                      Update kubernetes config."
	echo "   html                      Create html template."
	echo "   h/-h/help/-help/--help    Show this message."
}

pb() {
	rm ./api/*.pb.go
	rm ./api/*.md
	rm ./api/*.ts
	go mod tidy
	codegen -update
	corelib=$(go list -m -f {{"\"{{.Dir}}\""}} github.com/chenjie199234/Corelib)
	protoc -I ./ -I $corelib --go_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I $corelib --go-pbex_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I $corelib --go-cgrpc_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I $corelib --go-crpc_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I $corelib --go-web_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I $corelib --browser_out=paths=source_relative,gen_tob=true:. ./api/*.proto
	protoc -I ./ -I $corelib --markdown_out=paths=source_relative:. ./api/*.proto
	go mod tidy
}

sub() {
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -sub $1
}

kube() {
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -kube
}

html() {
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -html
}

if !(type git >/dev/null 2>&1);then
	echo "missing dependence: git"
	exit 0
fi

if !(type go >/dev/null 2>&1);then
	echo "missing dependence: golang"
	exit 0
fi

if !(type protoc >/dev/null 2>&1);then
	echo "missing dependence: protoc"
	exit 0
fi

if !(type protoc-gen-go >/dev/null 2>&1);then
	echo "missing dependence: protoc-gen-go"
	exit 0
fi

if !(type codegen >/dev/null 2>&1);then
	echo "missing dependence: codegen"
	exit 0
fi

if [[ $# == 0 ]] || [[ "$1" == "h" ]] || [[ "$1" == "help" ]] || [[ "$1" == "-h" ]] || [[ "$1" == "-help" ]] || [[ "$1" == "--help" ]]; then
	help
	exit 0
fi

if [[ "$1" == "pb" ]];then
	pb
	exit 0
fi

if [[ "$1" == "kube" ]];then
	kube
	exit 0
fi

if [[ "$1" == "html" ]];then
	html
	exit 0
fi

if [[ $# == 2 ]] && [[ "$1" == "sub" ]];then
	sub $2
	exit 0
fi

echo "option unsupport"
help`
const txtbat = `@echo off
REM      Warning!!!!!!!!!!!This file is readonly!Don't modify this file!

cd %~dp0

where /q git.exe
if %errorlevel% == 1 (
	echo "missing dependence: git"
	goto :end
)

where /q go.exe
if %errorlevel% == 1 (
	echo "missing dependence: golang"
	goto :end
)

where /q protoc.exe
if %errorlevel% == 1 (
	echo "missing dependence: protoc"
	goto :end
)

where /q protoc-gen-go.exe
if %errorlevel% == 1 (
	echo "missing dependence: protoc-gen-go"
	goto :end
)

where /q codegen.exe
if %errorlevel% == 1 (
	echo "missing dependence: codegen"
	goto :end
)

if "%1" == "" (
	goto :help
)
if %1 == "" (
	goto :help
)
if %1 == "h" (
	goto :help
)
if "%1" == "h" (
	goto :help
)
if %1 == "-h" (
	goto :help
)
if "%1" == "-h" (
	goto :help
)
if %1 == "help" (
	goto :help
)
if "%1" == "help" (
	goto :help
)
if %1 == "-help" (
	goto :help
)
if "%1" == "-help" (
	goto :help
)
if %1 == "pb" (
	goto :pb
)
if "%1" == "pb" (
	goto :pb
)
if %1 == "kube" (
	goto :kube
)
if "%1" ==  "kube" (
	goto :kube
)
if %1 == "html" (
	goto :html
)
if "%1" ==  "html" (
	goto :html
)
if %1 == "sub" (
	if "%2" == "" (
		goto :help
	)
	if %2 == "" (
		goto :help
	)
	goto :sub
)
if "%1" == "sub" (
	if "%2" == "" (
		goto :help
	)
	if %2 == "" (
		goto :help
	)
	goto :sub
)

goto :help

:pb
	del >nul 2>nul .\api\*.pb.go
	del >nul 2>nul .\api\*.md
	del >nul 2>nul .\api\*.ts
	go mod tidy
	codegen -update
	for /f %%a in ('go list -m -f {{"\"{{.Dir}}\""}} github.com/chenjie199234/Corelib') do set corelib=%%a
	protoc -I ./ -I %corelib% --go_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I %corelib% --go-pbex_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I %corelib% --go-cgrpc_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I %corelib% --go-crpc_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I %corelib% --go-web_out=paths=source_relative:. ./api/*.proto
	protoc -I ./ -I %corelib% --browser_out=paths=source_relative,gen_tob=true:. ./api/*.proto
	protoc -I ./ -I %corelib% --markdown_out=paths=source_relative:. ./api/*.proto
	go mod tidy
goto :end

:kube
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -kube
goto :end

:html
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -html
goto :end

:sub
	codegen -update
	codegen -n {{.ProjectName}} -p {{.PackageName}} -sub %2
goto :end

:help
	echo cmd.bat - every thing you need
	echo           please install git
	echo           please install golang(1.21+)
	echo           please install protoc           (github.com/protocolbuffers/protobuf)
	echo           please install protoc-gen-go    (github.com/protocolbuffers/protobuf-go)
	echo           please install codegen          (github.com/chenjie199234/Corelib)
	echo.
	echo Usage:
	echo    ./cmd.bat ^<option^>
	echo.
	echo Options:
	echo    pb                        Generate the proto in this program.
	echo    sub ^<sub service name^>    Create a new sub service.
	echo    kube                      Update kubernetes config.
	echo    html                      Create html template.
	echo    h/-h/help/-help/--help    Show this message.
:end
pause
exit /b 0`

type data struct {
	PackageName string
	ProjectName string
}

func CreatePathAndFile(packagename, projectname string) {
	tmp := &data{
		PackageName: packagename,
		ProjectName: projectname,
	}
	//./cmd.sh
	bashtemplate, e := template.New("./cmd.sh").Parse(txtbash)
	if e != nil {
		panic("parse ./cmd.sh error: " + e.Error())
	}
	bashfile, e := os.OpenFile("./cmd.sh", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0755)
	if e != nil {
		panic("open ./cmd.sh error: " + e.Error())
	}
	if e := bashtemplate.Execute(bashfile, tmp); e != nil {
		panic("write ./cmd.sh error: " + e.Error())
	}
	if e := bashfile.Sync(); e != nil {
		panic("sync ./cmd.sh error: " + e.Error())
	}
	if e := bashfile.Close(); e != nil {
		panic("close ./cmd.sh error: " + e.Error())
	}
	//./cmd.bat
	battemplate, e := template.New("./cmd.bat").Parse(strings.Replace(txtbat, "\n", "\r\n", -1))
	if e != nil {
		panic("parse ./cmd.bat error: " + e.Error())
	}
	batfile, e := os.OpenFile("./cmd.bat", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("parse ./cmd.bat error: " + e.Error())
	}
	if e := battemplate.Execute(batfile, tmp); e != nil {
		panic("write ./cmd.bat error: " + e.Error())
	}
	if e := batfile.Sync(); e != nil {
		panic("sync ./cmd.bat error: " + e.Error())
	}
	if e := batfile.Close(); e != nil {
		panic("close ./cmd.bat error: " + e.Error())
	}
}
