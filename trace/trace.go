package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/host"
)

var target int //1-std,2-file,3-both
var rf *rotatefile.RotateFile

func getenv() {
	temp := os.Getenv("TRACE_TARGET")
	if temp != "std" && temp != "file" && temp != "both" && temp != "" {
		panic("[trace] os env TRACE_TARGET error,must in [std(default),file,both]")
	}
	switch temp {
	case "":
		//default std
		fallthrough
	case "std":
		target = 1
	case "file":
		target = 2
	case "both":
		target = 3
	}
}
func init() {
	rand.Seed(time.Now().UnixNano())
	getenv()
	if target&2 > 0 {
		var e error
		rf, e = rotatefile.NewRotateFile("./log", "trace")
		if e != nil {
			panic("[trace] create rotate trace file error:" + e.Error())
		}
	}
}

type ROLE string

const (
	CLIENT ROLE = "client"
	SERVER ROLE = "server"
)

type TraceLog struct {
	TraceId    string `json:"trace_id"` //the whole trace route
	Start      int64  `json:"start"`    //nanosecond
	End        int64  `json:"end"`      //nanosecond
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

func InitTrace(ctx context.Context, traceid, app, ip, method, path string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		if app == "" || ip == "" || method == "" || path == "" {
			panic("[trace] init error: missing params")
		}
		if traceid == "" {
			traceid = maketraceid()
		}
		return context.WithValue(ctx, tracekey{}, map[string]string{"Traceid": traceid, "App": app, "Ip": ip, "Method": method, "Path": path})
	}
	return ctx
}

func GetTrace(ctx context.Context) (traceid, curapp, curip, curmethod, curpath string) {
	if ctx == nil {
		return
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		return
	}
	tracedata, _ := tmp.(map[string]string)
	traceid = tracedata["Traceid"]
	curapp = tracedata["App"]
	curip = tracedata["Ip"]
	curmethod = tracedata["Method"]
	curpath = tracedata["Path"]
	return
}

func CopyTrace(src context.Context) context.Context {
	if src == nil {
		return context.Background()
	}
	traceid, fromapp, fromip, frommethod, frompath := GetTrace(src)
	if traceid == "" {
		return context.Background()
	}
	return InitTrace(context.Background(), traceid, fromapp, fromip, frommethod, frompath)
}

func maketraceid() string {
	nowstr := strconv.FormatInt(time.Now().UnixNano(), 10)
	ranstr := strconv.FormatInt(rand.Int63(), 10)
	return nowstr + "_" + ranstr
}

func Trace(ctx context.Context, role ROLE, toapp, toip, tomethod, topath string, start, end *time.Time, e error) {
	traceid, fromapp, fromip, frommethod, frompath := GetTrace(ctx)
	if traceid == "" {
		return
	}
	ecode := int32(0)
	emsg := ""
	if ee := cerror.ConvertStdError(e); ee != nil {
		ecode = int32(ee.Code)
		emsg = ee.Msg
	}
	tracelog, _ := json.Marshal(&TraceLog{
		TraceId:    traceid,
		Start:      start.UnixNano(),
		End:        end.UnixNano(),
		HostName:   host.Hostname,
		Role:       string(role),
		FromApp:    fromapp,
		FromIP:     fromip,
		FromMethod: frommethod,
		FromPath:   frompath,
		ToApp:      toapp,
		ToIP:       toip,
		ToMethod:   tomethod,
		ToPath:     topath,
		ErrCode:    ecode,
		ErrMsg:     emsg,
	})
	write(tracelog)
}
func write(log []byte) {
	buf := bufpool.GetBuffer()
	buf.AppendString("[TRACE] ")
	buf.AppendByteSlice(log)
	buf.AppendByte('\n')
	if target&1 > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if target&2 > 0 {
		if _, e := rf.WriteBuf(buf); e != nil {
			fmt.Printf("[trace] write rotate file error: %s with data: %s\n", e, buf.String())
			bufpool.PutBuffer(buf)
		}
	} else {
		bufpool.PutBuffer(buf)
	}
}

func LogFileSize() int64 {
	return rf.GetCurFileLen()
}
func RotateLogFile() {
	if target&2 > 0 && rf != nil {
		if e := rf.RotateNow(); e != nil {
			fmt.Printf("[trace] rotate trace file error:%s\n", e)
		}
	}
}
func CleanLogFile(lastModTimestampBeforeThisNS int64) {
	if target&2 > 0 && rf != nil {
		if e := rf.CleanNow(lastModTimestampBeforeThisNS); e != nil {
			fmt.Printf("[trace] clean trace file before timestamp:%dns error:%s\n", lastModTimestampBeforeThisNS, e)
		}
	}
}
func Close() {
	if target&2 > 0 && rf != nil {
		rf.Close()
	}
}
