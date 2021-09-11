package ecode

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package ecode

import (
	cerror "github.com/chenjie199234/Corelib/error"
)

var (
	ErrUnknown   = cerror.ErrUnknown //10000
	ErrReq       = cerror.ErrReq     //10001
	ErrResp      = cerror.ErrResp    //10002
	ErrSystem    = cerror.ErrSystem  //10003
	ErrAuth      = cerror.ErrAuth    //10004
	ErrLimit     = cerror.ErrLimit   //10005
	ErrBan       = cerror.ErrBan     //10006

	ErrBusiness1 = cerror.MakeError(20001, "business error 1")
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
