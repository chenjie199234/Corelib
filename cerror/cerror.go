package cerror

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/chenjie199234/Corelib/util/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

func MakeCError(code int32, httpcode int32, msg string) *Error {
	if code == 0 {
		panic("error code can't be 0")
	}
	if httpcode < 400 {
		panic("error's http code must >= 400")
	}
	if http.StatusText(int(httpcode)) == "" {
		panic("error's http code unknown")
	}
	e := &Error{}
	e.SetCode(code)
	e.SetHttpcode(httpcode)
	e.SetMsg(msg)
	return e
}
func (this *Error) Error() string {
	if this == nil {
		return ""
	}
	if len(this.GetCacheText()) == 0 {
		str := "code=" + strconv.FormatInt(int64(this.GetCode()), 10) + ",msg=" + this.GetMsg()
		this.SetCacheText(str)
	}
	return this.GetCacheText()
}
func (this *Error) Json() string {
	if this == nil {
		return ""
	}
	if len(this.GetCacheJson()) == 0 {
		d, _ := json.Marshal(this.GetMsg())
		str := "{\"code\":" + strconv.FormatInt(int64(this.GetCode()), 10) + ",\"msg\":" + common.BTS(d) + "}"
		this.SetCacheJson(str)
	}
	return this.GetCacheJson()
}
func (this *Error) CleanCache() {
	this.SetCacheText("")
	this.SetCacheJson("")
}
func (this *Error) GRPCStatus() *status.Status {
	return status.New(codes.Code(this.GetHttpcode()), this.Error())
}
func (this *Error) SlogAttr() *slog.Attr {
	return &slog.Attr{Key: "error", Value: slog.GroupValue(slog.Int64("code", int64(this.GetCode())), slog.String("msg", this.GetMsg()))}
}
func Equal(a, b error) bool {
	if a == b {
		return true
	}
	aa := Convert(a)
	bb := Convert(b)
	if aa == nil && bb == nil {
		return true
	} else if (aa == nil && bb != nil) || (aa != nil && bb == nil) {
		return false
	}
	return aa.GetCode() == bb.GetCode() && aa.GetMsg() == bb.GetMsg()
}
func Convert(e error) *Error {
	if e == nil {
		return nil
	}
	switch e {
	case context.DeadlineExceeded:
		return ErrDeadlineExceeded
	case context.Canceled:
		return ErrCanceled
	}
	result, ok := e.(*Error)
	if ok {
		return result
	}
	return MakeCError(-1, 500, e.Error())
}
func Decode(estr string) *Error {
	if estr == "" {
		return nil
	}
	if estr == ErrDeadlineExceeded.Json() || estr == ErrDeadlineExceeded.Error() {
		return ErrDeadlineExceeded
	} else if estr == ErrCanceled.Json() || estr == ErrCanceled.Error() {
		return ErrCanceled
	}
	if estr[0] == '{' && estr[len(estr)-1] == '}' {
		if len(estr) == 2 {
			return nil
		}
		//json format
		tmp := &Error{}
		//protojson can support "number string" or "number" for field:code
		if e := protojson.Unmarshal(common.STB(estr), tmp); e != nil {
			return MakeCError(-1, 500, estr)
		}
		if tmp.GetCode() == 0 ||
			(tmp.HasHttpcode() &&
				(tmp.GetHttpcode() < 400 ||
					http.StatusText(int(tmp.GetHttpcode())) == "")) {
			tmp.SetCode(-1)
			tmp.SetMsg(estr)
		}
		if tmp.GetHttpcode() == 0 {
			tmp.SetHttpcode(500)
		}
		return tmp
	}
	//text format
	p1, p2, ok := strings.Cut(estr, ",")
	if !ok {
		return MakeCError(-1, 500, estr)
	}
	if !strings.HasPrefix(p1, "code=") || !strings.HasPrefix(p2, "msg=") {
		return MakeCError(-1, 500, estr)
	}
	code, e := strconv.ParseInt(p1[5:], 10, 32)
	if e != nil {
		return MakeCError(-1, 500, estr)
	}
	msg := p2[4:]
	tmp := &Error{}
	if code == 0 {
		tmp.SetCode(-1)
		tmp.SetMsg(msg)
	} else {
		tmp.SetCode(int32(code))
		tmp.SetMsg(msg)
	}
	tmp.SetHttpcode(500)
	return tmp
}
