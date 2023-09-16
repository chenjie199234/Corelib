package ecode

import (
	"os"
)

const txt = `package ecode

import (
	"net/http"

	"github.com/chenjie199234/Corelib/cerror"
)

var (
	ErrServerClosing = cerror.ErrServerClosing //1000  // http code 449 Warning!! Client will retry on this error,be careful to use this error
	ErrUnknown       = cerror.ErrUnknown       //10000 // http code 500
	ErrReq           = cerror.ErrReq           //10001 // http code 400
	ErrResp          = cerror.ErrResp          //10002 // http code 500
	ErrSystem        = cerror.ErrSystem        //10003 // http code 500
	ErrToken         = cerror.ErrToken         //10004 // http code 401
	ErrSession       = cerror.ErrSession       //10005 // http code 401
	ErrKey           = cerror.ErrKey           //10006 // http code 401
	ErrSign          = cerror.ErrSign          //10007 // http code 401
	ErrPermission    = cerror.ErrPermission    //10008 // http code 403
	ErrTooFast       = cerror.ErrTooFast       //10009 // http code 403
	ErrBan           = cerror.ErrBan           //10010 // http code 403
	ErrBusy          = cerror.ErrBusy          //10011 // http code 503
	ErrNotExist      = cerror.ErrNotExist      //10012 // http code 404

	ErrBusiness1 = cerror.MakeError(20001, http.StatusBadRequest, "business error 1")
)

func ReturnEcode(originerror error, defaulterror *cerror.Error) error {
	if _, ok := originerror.(*cerror.Error); ok {
		return originerror
	}
	return defaulterror
}`

func CreatePathAndFile() {
	if e := os.MkdirAll("./ecode/", 0755); e != nil {
		panic("mkdir ./ecode/ error: " + e.Error())
	}
	file, e := os.OpenFile("./ecode/ecode.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./ecode/ecode.go error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./ecode/ecode.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./ecode/ecode.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./ecode/ecode.go error: " + e.Error())
	}
}
