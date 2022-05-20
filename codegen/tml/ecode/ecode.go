package ecode

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package ecode

import (
	"net/http"

	cerror "github.com/chenjie199234/Corelib/error"
)

var (
	ErrUnknown  = cerror.ErrUnknown  //10000
	ErrReq      = cerror.ErrReq      //10001 // http code 400
	ErrResp     = cerror.ErrResp     //10002 // http code 500
	ErrSystem   = cerror.ErrSystem   //10003 // http code 500
	ErrAuth     = cerror.ErrAuth     //10004 // http code 401
	ErrLimit    = cerror.ErrLimit    //10005 // http code 503
	ErrBan      = cerror.ErrBan      //10006 // http code 403
	ErrNotExist = cerror.ErrNotExist //10007 // http code 404
	ErrBusy     = cerror.ErrBusy     //10008 // http code 503

	ErrBusiness1 = cerror.MakeError(20001,http.StatusBadRequest, "business error 1")
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
