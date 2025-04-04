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
	return &Error{
		Code:     code,
		Httpcode: httpcode,
		Msg:      msg,
	}
}
func (this *Error) Error() string {
	return "code=" + strconv.FormatInt(int64(this.Code), 10) + ",msg=" + this.Msg
}
func (this *Error) Json() string {
	d, _ := json.Marshal(this.Msg)
	return "{\"code\":" + strconv.FormatInt(int64(this.Code), 10) + ",\"msg\":" + common.BTS(d) + "}"
}
func (this *Error) GRPCStatus() *status.Status {
	return status.New(codes.Code(this.Httpcode), this.Error())
}
func (this *Error) SlogAttr() *slog.Attr {
	return &slog.Attr{Key: "error", Value: slog.GroupValue(slog.Int64("code", int64(this.Code)), slog.String("msg", this.Msg))}
}
func (this *Error) SetHttpcode(httpcode int32) {
	this.Httpcode = httpcode
}
func Equal(a, b error) bool {
	aa := Convert(a)
	bb := Convert(b)
	if aa == nil && bb == nil {
		return true
	} else if (aa == nil && bb != nil) || (aa != nil && bb == nil) {
		return false
	}
	return aa.Code == bb.Code && aa.Msg == bb.Msg
}
func Convert(e error) *Error {
	if e == nil {
		return nil
	}
	if e == context.DeadlineExceeded {
		return ErrDeadlineExceeded
	} else if e == context.Canceled {
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
		if e := (protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}).Unmarshal(common.STB(estr), tmp); e != nil {
			return MakeCError(-1, 500, estr)
		}
		if tmp.Code == 0 {
			tmp.Code = -1
			tmp.Msg = estr
		}
		if tmp.Httpcode == 0 {
			tmp.Httpcode = 500
		}
		return tmp
	}
	//text format
	index := strings.Index(estr, ",")
	if index == -1 {
		return MakeCError(-1, 500, estr)
	}
	p1 := estr[:index]
	p2 := estr[index+1:]
	if !strings.HasPrefix(p1, "code=") || !strings.HasPrefix(p2, "msg=") {
		return MakeCError(-1, 500, estr)
	}
	code, e := strconv.ParseInt(p1[5:], 10, 32)
	if e != nil {
		return MakeCError(-1, 500, estr)
	}
	msg := p2[4:]
	return MakeCError(int32(code), 500, msg)
}
