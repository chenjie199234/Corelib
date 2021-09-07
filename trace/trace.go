package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
	cerror "github.com/chenjie199234/Corelib/util/error"
	"github.com/chenjie199234/Corelib/util/host"
)

var target int //1-std,2-file,3-both
var rf *rotatefile.RotateFile

func getenv() {
	if os.Getenv("TRACE_TARGET") == "" {
		//default std
		target = 1
	} else {
		temp := strings.ToLower(os.Getenv("LOG_TARGET"))
		if temp != "std" && temp != "file" && temp != "both" {
			panic("[trace] os env TRACE_TARGET error,must in [std(default),file,both]")
		}
		switch temp {
		case "std":
			target = 1
		case "file":
			target = 2
		case "both":
			target = 3
		}
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

type KIND string

const (
	RPC   KIND = "rpc"
	WEB   KIND = "web"
	MYSQL KIND = "mysql"
	MONGO KIND = "mongo"
	REDIS KIND = "redis"
	KAFKA KIND = "kafka"
)

type ACTION string

const (
	START ACTION = "start"
	END   ACTION = "end"
)

type TraceLog struct {
	TraceId    string `json:"trace_id"`     //the whole trace route
	SubTraceId string `json:"sub_trace_id"` //each sub trace id will have two log,one's action is start,another is end
	Action     string `json:"action"`       //start or end
	Timestamp  int64  `json:"timestamp"`    //nanosecond
	HostName   string `json:"host_name"`
	Role       string `json:"role"`
	FromApp    string `json:"from_app"`
	FromIP     string `json:"from_ip"`
	FromMethod string `json:"from_method"`
	FromPath   string `json:"from_path"`
	FromKind   string `json:"from_kind"`
	ToApp      string `json:"to_app"`
	ToIP       string `json:"to_ip"`
	ToMethod   string `json:"to_method"`
	ToPath     string `json:"to_path"`
	ToKind     string `json:"to_kind"`
	ErrCode    int32  `json:"err_code"`
	ErrMsg     string `json:"err_msg"`
}

type tracekey struct{}

func InitTrace(ctx context.Context, traceid, app, ip, method, path string, kind KIND) context.Context {
	if app == "" || ip == "" || method == "" || path == "" || kind == "" {
		panic("[trace] init error: missing params")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		if traceid == "" {
			traceid = maketraceid()
		}
		return context.WithValue(ctx, tracekey{}, map[string]string{"Traceid": traceid, "App": app, "Ip": ip, "Method": method, "Path": path, "Kind": string(kind)})
	}
	return ctx
}

func GetTrace(ctx context.Context) (traceid, app, ip, method, path string, kind KIND) {
	if ctx == nil {
		return
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		return
	}
	tracedata, _ := tmp.(map[string]string)
	traceid = tracedata["Traceid"]
	app = tracedata["App"]
	ip = tracedata["Ip"]
	method = tracedata["Method"]
	path = tracedata["Path"]
	kind = KIND(tracedata["Kind"])
	return
}

func GetTraceMap(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		return nil
	}
	tracedata, _ := tmp.(map[string]string)
	return tracedata
}

func MapToTrace(tracedata map[string]string) (traceid, app, ip, method, path string, kind KIND) {
	if tracedata == nil {
		return
	}
	traceid = tracedata["Traceid"]
	if traceid == "" {
		return
	}
	app = tracedata["App"]
	ip = tracedata["Ip"]
	method = tracedata["Method"]
	path = tracedata["Path"]
	kind = KIND(tracedata["Kind"])
	return
}

func maketraceid() string {
	nowstr := strconv.FormatInt(time.Now().UnixNano(), 10)
	ranstr := strconv.FormatInt(rand.Int63(), 10)
	return nowstr + "_" + ranstr
}
func TraceStart(ctx context.Context, role ROLE, fromapp, fromip, frommethod, frompath string, fromkind KIND, toapp, toip, tomethod, topath string, tokind KIND) (TraceEnd func(e error)) {
	if ctx == nil {
		return nil
	}
	tmp := ctx.Value(tracekey{})
	if tmp == nil {
		return nil
	}
	traceid := (tmp.(map[string]string))["Traceid"]
	subtraceid := maketraceid()
	startlog, _ := json.Marshal(&TraceLog{
		TraceId:    traceid,
		SubTraceId: subtraceid,
		Action:     string(START),
		Timestamp:  time.Now().UnixNano(),
		HostName:   host.Hostname,
		Role:       string(role),
		FromApp:    fromapp,
		FromIP:     fromip,
		FromMethod: frommethod,
		FromPath:   frompath,
		FromKind:   string(fromkind),
		ToApp:      toapp,
		ToIP:       toip,
		ToMethod:   tomethod,
		ToPath:     topath,
		ToKind:     string(tokind),
	})
	write(startlog)
	return func(e error) {
		tmp := &TraceLog{
			TraceId:    traceid,
			SubTraceId: subtraceid,
			Action:     string(END),
			Timestamp:  time.Now().UnixNano(),
			HostName:   host.Hostname,
			Role:       string(role),
			FromApp:    fromapp,
			FromIP:     fromip,
			FromMethod: frommethod,
			FromPath:   frompath,
			FromKind:   string(fromkind),
			ToApp:      toapp,
			ToIP:       toip,
			ToMethod:   tomethod,
			ToPath:     topath,
			ToKind:     string(tokind),
		}
		if ee := cerror.StdErrorToError(e); ee != nil {
			tmp.ErrCode = ee.Code
			tmp.ErrMsg = ee.Msg
		}
		endlog, _ := json.Marshal(tmp)
		write(endlog)
	}
}
func write(log []byte) {
	buf := bufpool.GetBuffer()
	buf.Append("[TRACE] ")
	buf.Append(common.Byte2str(log))
	buf.Append("\n")
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

func RotateLogFileSize() int64 {
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
