package log

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var trace bool
var target int //1-std(default),2-file,3-both
var level int  //0-debug,1-info(default),2-warning,3-error
var rf *rotatefile.RotateFile

func getenv() {
	if str := os.Getenv("LOG_TRACE"); str != "" && str != "<LOG_TRACE>" && str != "0" && str != "1" {
		panic("[log] os env LOG_TRACE error,must in [0,1]")
	} else {
		trace = str == "1"
	}
	if str := strings.ToLower(os.Getenv("LOG_TARGET")); str != "std" && str != "file" && str != "both" && str != "" && str != "<LOG_TARGET>" {
		panic("[log] os env LOG_TARGET error,must in [std(default),file,both]")
	} else {
		switch str {
		case "<LOG_TARGET>":
			//default std
			fallthrough
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
	if str := strings.ToLower(os.Getenv("LOG_LEVEL")); str != "debug" && str != "info" && str != "warning" && str != "error" && str != "" && str != "<LOG_LEVEL>" {
		panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warning,error]")
	} else {
		switch str {
		case "debug":
			level = 0
		case "<LOG_LEVEL>":
			//default info
			fallthrough
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
}

func init() {
	getenv()
	if target&2 > 0 {
		var e error
		path, e := os.Executable()
		if e != nil {
			panic("[log] get current exec file's path error:" + e.Error())
		}
		logdir := ""
		if index := strings.LastIndex(path, "/"); index == -1 {
			index = strings.LastIndex(path, "\\")
			if index != -1 {
				logdir = path[:index] + "\\log"
			} else {
				panic("[log] can't get log dir path")
			}
		} else {
			logdir = path[:index] + "/log"
		}
		rf, e = rotatefile.NewRotateFile(logdir, "log")
		if e != nil {
			panic("[log] create rotate file error:" + e.Error())
		}
	}
}
func Debug(ctx context.Context, summary string, kvs map[string]interface{}) {
	if level > 0 {
		return
	}
	write(ctx, "DEBUG", summary, kvs)
}
func Info(ctx context.Context, summary string, kvs map[string]interface{}) {
	if level > 1 {
		return
	}
	write(ctx, "INFO", summary, kvs)
}
func Warning(ctx context.Context, summary string, kvs map[string]interface{}) {
	if level > 2 {
		return
	}
	write(ctx, "WARN", summary, kvs)
}
func Error(ctx context.Context, summary string, kvs map[string]interface{}) {
	write(ctx, "ERROR", summary, kvs)
}
func write(ctx context.Context, lv, summary string, kvs map[string]interface{}) {
	buf := pool.GetBuffer()
	buf.AppendString("{\"_lv\":\"")
	buf.AppendString(lv)
	buf.AppendString("\",\"_summary\":\"")
	buf.AppendString(summary)
	buf.AppendString("\",\"_time\":\"")
	buf.AppendStdTime(time.Now().UTC())
	buf.AppendString("\",\"_fileline\":\"")
	_, file, line, _ := runtime.Caller(2)
	buf.AppendString(file)
	buf.AppendByte(':')
	buf.AppendInt(line)
	buf.AppendByte('"')
	if trace {
		traceid, _, _, _, _, deep := GetTrace(ctx)
		if traceid != "" {
			buf.AppendString(",\"_tid\":\"")
			buf.AppendString(traceid)
			buf.AppendString("\",\"_tdeep\":")
			buf.AppendInt(deep)
		}
	}
	if len(kvs) != 0 {
		buf.AppendString(",\"_kvs\":{")
		first := true
		for k, v := range kvs {
			if !first {
				buf.AppendString(",")
			}
			special := false
			for _, vv := range k {
				if vv == '\\' || vv == '"' {
					special = true
					break
				}
			}
			if special {
				kk, _ := json.Marshal(k)
				buf.AppendBytes(kk)
				buf.AppendByte(':')
			} else {
				buf.AppendByte('"')
				buf.AppendString(k)
				buf.AppendString("\":")
			}
			writeany(buf, v)
			first = false
		}
		buf.AppendString("}}\n")
	} else {
		buf.AppendString("}\n")
	}
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
		special := false
		for _, v := range d {
			if v == '\\' || v == '"' {
				special = true
				break
			}
		}
		if special {
			dd, _ := json.Marshal(d)
			buf.AppendBytes(dd)
		} else {
			buf.AppendByte('"')
			buf.AppendString(d)
			buf.AppendByte('"')
		}
	case []string:
		for i, v := range d {
			special := false
			for _, vv := range v {
				if vv == '\\' || vv == '"' {
					special = true
					break
				}
			}
			if special {
				vv, _ := json.Marshal(v)
				d[i] = common.Byte2str(vv)
			}
		}
		buf.AppendStrings(d)

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
	case uint8:
		buf.AppendUint8(d)
	case []uint8:
		buf.AppendUint8s(d)

	case int16:
		buf.AppendInt16(d)
	case []int16:
		buf.AppendInt16s(d)
	case uint16:
		buf.AppendUint16(d)
	case []uint16:
		buf.AppendUint16s(d)

	case ctime.Duration:
		buf.AppendByte('"')
		buf.AppendDuration(d)
		buf.AppendByte('"')
	case *ctime.Duration:
		buf.AppendByte('"')
		buf.AppendDuration(*d)
		buf.AppendByte('"')
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
		buf.AppendByte('"')
		buf.AppendStdTime(d)
		buf.AppendByte('"')
	case *time.Time:
		buf.AppendByte('"')
		buf.AppendStdTime(*d)
		buf.AppendByte('"')
	case []time.Time:
		buf.AppendStdTimes(d)
	case []*time.Time:
		buf.AppendStdTimePointers(d)

	case *cerror.Error:
		buf.AppendError(d)
	case []*cerror.Error:
		buf.AppendErrors(d)
	case error:
		estr := d.Error()
		special := false
		for _, v := range estr {
			if v == '\\' || v == '"' {
				special = true
				break
			}
		}
		if special {
			dd, _ := json.Marshal(estr)
			buf.AppendBytes(dd)
		} else {
			buf.AppendByte('"')
			buf.AppendString(estr)
			buf.AppendByte('"')
		}
	case []error:
		buf.AppendByte('[')
		for i, v := range d {
			if i != 0 {
				buf.AppendByte(',')
			}
			if v == nil {
				buf.AppendString("null")
			} else if vv, ok := v.(*cerror.Error); ok {
				buf.AppendError(vv)
			} else {
				estr := v.Error()
				special := false
				for _, vvv := range estr {
					if vvv == '\\' || vvv == '"' {
						special = true
						break
					}
				}
				if special {
					dd, _ := json.Marshal(estr)
					buf.AppendBytes(dd)
				} else {
					buf.AppendByte('"')
					buf.AppendString(estr)
					buf.AppendByte('"')
				}
			}
		}
		buf.AppendByte(']')

	default:
		if d, ok := data.(protoreflect.ProtoMessage); ok {
			tmp, e := protojson.Marshal(d)
			if e != nil {
				buf.AppendString("unsupported type")
			} else {
				buf.AppendBytes(tmp)
			}
		} else {
			tmp, e := json.Marshal(data)
			if e != nil {
				buf.AppendString("unsupported type")
			} else {
				buf.AppendBytes(tmp)
			}
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
