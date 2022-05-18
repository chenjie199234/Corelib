package log

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/host"
)

type ROLE string

const (
	CLIENT ROLE = "client"
	SERVER ROLE = "server"
)

type TraceLog struct {
	TraceId    string `json:"trace_id"` //the whole trace route
	Deep       int    `json:"deep"`
	Start      int64  `json:"start"` //nanosecond
	End        int64  `json:"end"`   //nanosecond
	HostName   string `json:"host_name"`
	Role       string `json:"role"`
	FromApp    string `json:"from_app"`
	FromIP     string `json:"from_ip"`
	FromMethod string `json:"from_method"`
	FromPath   string `json:"from_path"`
	ToApp      string `json:"to_app"`
	ToIP       string `json:"to_ip"`
	ToMethod   string `json:"to_method"`
	ToPath     string `json:"to_path"`
	ErrCode    int32  `json:"err_code"`
	ErrMsg     string `json:"err_msg"`
}

type tracekey struct{}

func InitTrace(ctx context.Context, traceid, app, ip, method, path string, deep int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	tmp, ok := ctx.Value(tracekey{}).(map[string]string)
	if !ok || tmp["Traceid"] == "" {
		if app == "" || ip == "" || method == "" || path == "" {
			panic("[trace] init error: missing params")
		}
		if traceid == "" {
			deep = 0
			traceid = maketraceid()
		}
		deep++
		return context.WithValue(ctx, tracekey{}, map[string]string{"Traceid": traceid, "Deep": strconv.Itoa(deep), "App": app, "Ip": ip, "Method": method, "Path": path})
	}
	return ctx
}

func GetTrace(ctx context.Context) (traceid, curapp, curip, curmethod, curpath string, curdeep int) {
	if ctx == nil {
		return
	}
	tracedata, ok := ctx.Value(tracekey{}).(map[string]string)
	if !ok || tracedata["Traceid"] == "" {
		return
	}
	traceid = tracedata["Traceid"]
	curapp = tracedata["App"]
	curip = tracedata["Ip"]
	curmethod = tracedata["Method"]
	curpath = tracedata["Path"]
	curdeep, _ = strconv.Atoi(tracedata["Deep"])
	return
}

//this will overwrite dst's tracedata
func CopyTrace(src, dst context.Context) (context.Context, bool) {
	if dst == nil {
		dst = context.Background()
	}
	if src == nil {
		return dst, false
	}
	tracedata, ok := src.Value(tracekey{}).(map[string]string)
	if ok && tracedata["Traceid"] != "" {
		return context.WithValue(dst, tracekey{}, tracedata), true
	}
	return dst, false
}

func maketraceid() string {
	nowstr := strconv.FormatInt(time.Now().UnixNano(), 10)
	ranstr := strconv.FormatInt(rand.Int63(), 10)
	return nowstr + "_" + ranstr
}

func Trace(ctx context.Context, role ROLE, toapp, toip, tomethod, topath string, start, end *time.Time, e error) {
	if !trace {
		return
	}
	traceid, fromapp, fromip, frommethod, frompath, deep := GetTrace(ctx)
	if traceid == "" {
		return
	}
	ecode := int32(0)
	emsg := ""
	if ee := cerror.ConvertStdError(e); ee != nil {
		ecode = int32(ee.Code)
		emsg = ee.Msg
	}
	buf := pool.GetBuffer()
	buf.AppendString("[TRC] {\"trace_id\":\"")
	buf.AppendString(traceid)
	buf.AppendString("\",\"deep\":")
	buf.AppendInt(deep)
	buf.AppendString(",\"start\":")
	buf.AppendInt64(start.UnixNano())
	buf.AppendString(",\"end\":")
	buf.AppendInt64(end.UnixNano())
	buf.AppendString(",\"host_name\":\"")
	buf.AppendString(host.Hostname)
	buf.AppendString("\",\"role\":\"")
	buf.AppendString(string(role))
	buf.AppendString("\",\"from_app\":\"")
	buf.AppendString(fromapp)
	buf.AppendString("\",\"from_ip\":\"")
	buf.AppendString(fromip)
	buf.AppendString("\",\"from_method\":\"")
	buf.AppendString(frommethod)
	buf.AppendString("\",\"from_path\":\"")
	buf.AppendString(frompath)
	buf.AppendString("\",\"to_app\":\"")
	buf.AppendString(toapp)
	buf.AppendString("\",\"to_ip\":\"")
	buf.AppendString(toip)
	buf.AppendString("\",\"to_method\":\"")
	buf.AppendString(tomethod)
	buf.AppendString("\",\"to_path\":\"")
	buf.AppendString(topath)
	buf.AppendString("\",\"err_msg\":\"")
	buf.AppendString(emsg)
	buf.AppendString("\",\"err_code\":")
	buf.AppendInt32(ecode)
	buf.AppendString("}\n")
	if target&1 > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if target&2 > 0 {
		if _, e := rf.WriteBuf(buf); e != nil {
			fmt.Printf("[trace] write rotate file error: %s with data: %s\n", e, buf.String())
			pool.PutBuffer(buf)
		}
	} else {
		pool.PutBuffer(buf)
	}
}
