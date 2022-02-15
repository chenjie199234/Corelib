package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/trace"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

var target int //1-std(default),2-file,3-both
var level int  //0-debug,1-info(default),2-warning,3-error
var rf *rotatefile.RotateFile

func getenv() {
	temp := strings.ToLower(os.Getenv("LOG_TARGET"))
	if temp != "std" && temp != "file" && temp != "both" && temp != "" {
		panic("[log] os env LOG_TARGET error,must in [std(default),file,both]")
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
	temp = strings.ToLower(os.Getenv("LOG_LEVEL"))
	if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" && temp != "" {
		panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warning,error]")
	}
	switch temp {
	case "debug":
		level = 0
	case "":
		//default info
		fallthrough
	case "info":
		level = 1
	case "warning":
		level = 2
	case "error":
		level = 3
	}
}

func init() {
	getenv()
	if target&2 > 0 {
		var e error
		rf, e = rotatefile.NewRotateFile("./log", "log")
		if e != nil {
			panic("[log] create rotate file error:" + e.Error())
		}
	}
}
func Debug(ctx context.Context, datas ...interface{}) {
	if level > 0 {
		return
	}
	buf := pool.GetBuffer()
	buf.AppendString("[DBG] ")
	write(ctx, buf, datas...)
}
func Info(ctx context.Context, datas ...interface{}) {
	if level > 1 {
		return
	}
	buf := pool.GetBuffer()
	buf.AppendString("[INF] ")
	write(ctx, buf, datas...)
}
func Warning(ctx context.Context, datas ...interface{}) {
	if level > 2 {
		return
	}
	buf := pool.GetBuffer()
	buf.AppendString("[WRN] ")
	write(ctx, buf, datas...)
}
func Error(ctx context.Context, datas ...interface{}) {
	buf := pool.GetBuffer()
	buf.AppendString("[ERR] ")
	write(ctx, buf, datas...)
}
func write(ctx context.Context, buf *pool.Buffer, datas ...interface{}) {
	buf.AppendStdTime(time.Now())
	_, file, line, _ := runtime.Caller(2)
	buf.AppendByte(' ')
	buf.AppendString(file)
	buf.AppendByte(':')
	buf.AppendInt(line)
	traceid, _, _, _, _, deep := trace.GetTrace(ctx)
	if traceid != "" {
		buf.AppendByte(' ')
		buf.AppendString("Traceid: ")
		buf.AppendString(traceid)
		buf.AppendByte(' ')
		buf.AppendString("Tracedeep: ")
		buf.AppendInt(deep)
	}
	for _, data := range datas {
		buf.AppendByte(' ')
		writeany(buf, data)
	}
	buf.AppendByte('\n')
	if target&1 > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if target&2 > 0 {
		if _, e := rf.WriteBuf(buf); e != nil {
			fmt.Printf("[log] write rotate file error: %s with data: %s\n", e, buf.String())
			pool.PutBuffer(buf)
		}
	} else {
		pool.PutBuffer(buf)
	}
}
func writeany(buf *pool.Buffer, data interface{}) {
	switch d := data.(type) {
	case string:
		buf.AppendString(d)
	case []string:
		buf.AppendStrings(d)
	//case []uint8:
	case []byte:
		buf.AppendByteSlice(d)
	case [][]byte:
		buf.AppendByteSlices(d)
	//case uint8:
	case byte:
		buf.AppendByte(d)

	case int64:
		buf.AppendInt64(d)
	case []int64:
		buf.AppendInt64s(d)
	case uint64:
		buf.AppendUint64(d)
	case []uint64:
		buf.AppendUint64s(d)
	case float64:
		buf.AppendFloat64(d)
	case []float64:
		buf.AppendFloat64s(d)

	case int32:
		buf.AppendInt32(d)
	case []int32:
		buf.AppendInt32s(d)
	case uint32:
		buf.AppendUint32(d)
	case []uint32:
		buf.AppendUint32s(d)
	case float32:
		buf.AppendFloat32(d)
	case []float32:
		buf.AppendFloat32s(d)

	case int:
		buf.AppendInt(d)
	case []int:
		buf.AppendInts(d)
	case uint:
		buf.AppendUint(d)
	case []uint:
		buf.AppendUints(d)

	case int8:
		buf.AppendInt8(d)
	case []int8:
		buf.AppendInt8s(d)

	case int16:
		buf.AppendInt16(d)
	case []int16:
		buf.AppendInt16s(d)
	case uint16:
		buf.AppendUint16(d)
	case []uint16:
		buf.AppendUint16s(d)

	case ctime.Duration:
		buf.AppendDuration(d)
	case *ctime.Duration:
		buf.AppendDuration(*d)
	case []ctime.Duration:
		buf.AppendDurations(d)
	case []*ctime.Duration:
		buf.AppendDurationPointers(d)

	case time.Duration:
		buf.AppendStdDuration(d)
	case *time.Duration:
		buf.AppendStdDuration(*d)
	case []time.Duration:
		buf.AppendStdDurations(d)
	case []*time.Duration:
		buf.AppendStdDurationPointers(d)

	case time.Time:
		buf.AppendStdTime(d)
	case *time.Time:
		buf.AppendStdTime(*d)
	case []time.Time:
		buf.AppendStdTimes(d)
	case []*time.Time:
		buf.AppendStdTimePointers(d)

	case error:
		buf.AppendError(d)
	case []error:
		buf.AppendErrors(d)

	default:
		tmp, e := json.Marshal(data)
		if e != nil {
			buf.AppendString("unsupported type")
		} else {
			buf.AppendByteSlice(tmp)
		}
	}
}
func LogFileSize() int64 {
	return rf.GetCurFileLen()
}
func RotateLogFile() {
	if target&2 > 0 && rf != nil {
		if e := rf.RotateNow(); e != nil {
			fmt.Printf("[log] rotate log file error:%s\n", e)
		}
	}
}
func CleanLogFile(lastModTimestampBeforeThisNS int64) {
	if target&2 > 0 && rf != nil {
		if e := rf.CleanNow(lastModTimestampBeforeThisNS); e != nil {
			fmt.Printf("[log] clean log file before timestamp:%dns error:%s\n", lastModTimestampBeforeThisNS, e)
		}
	}
}
func Close() {
	if target&2 > 0 && rf != nil {
		rf.Close()
	}
}
