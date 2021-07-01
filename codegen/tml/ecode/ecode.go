package ecode

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package ecode

import (
	errors

	cerror "github.com/chenjie199234/Corelib/util/error"
)

var (
	ErrUnknown = errors.New(cerror.MakeError(10000, "unknown error").Error())
	ErrSystem  = errors.New(cerror.MakeError(10001, "system error").Error())
	ErrParams  = errors.New(cerror.MakeError(10002, "params error").Error())
)`

const path = "./ecode/"
const filename = "ecode.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("ecode").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+filename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+filename, e))
	}
}
func Execute() {
	if e := tml.Execute(file, nil); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+filename, e))
	}
}
