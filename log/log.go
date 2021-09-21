package log

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/trace"
)

var target int //1-std(default),2-file,3-both
var level int  //0-debug,1-info(default),2-warning,3-error
var rf *rotatefile.RotateFile

func getenv() {
	if os.Getenv("LOG_TARGET") == "" {
		//default std
		target = 1
	} else {
		temp := strings.ToLower(os.Getenv("LOG_TARGET"))
		if temp != "std" && temp != "file" && temp != "both" {
			panic("[log] os env LOG_TARGET error,must in [std(default),file,both]")
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
	if os.Getenv("LOG_LEVEL") == "" {
		//default info
		level = 1
	} else {
		temp := strings.ToLower(os.Getenv("LOG_LEVEL"))
		if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" {
			panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warning,error]")
		}
		switch temp {
		case "debug":
			level = 0
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
	buf := bufpool.GetBuffer()
	buf.Append("[DBG] ")
	write(ctx, buf, datas...)
}
func Info(ctx context.Context, datas ...interface{}) {
	if level > 1 {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[INF] ")
	write(ctx, buf, datas...)
}
func Warning(ctx context.Context, datas ...interface{}) {
	if level > 2 {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[WRN] ")
	write(ctx, buf, datas...)
}
func Error(ctx context.Context, datas ...interface{}) {
	buf := bufpool.GetBuffer()
	buf.Append("[ERR] ")
	write(ctx, buf, datas...)
}
func write(ctx context.Context, buf *bufpool.Buffer, datas ...interface{}) {
	buf.Append(time.Now())
	_, file, line, _ := runtime.Caller(2)
	buf.Append(" ")
	buf.Append(file)
	buf.Append(":")
	buf.Append(line)
	traceid, _, _, _, _ := trace.GetTrace(ctx)
	if traceid != "" {
		buf.Append(" ")
		buf.Append("Traceid: ")
		buf.Append(traceid)
	}
	for _, data := range datas {
		buf.Append(" ")
		buf.Append(data)
	}
	buf.Append("\n")
	if target&1 > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if target&2 > 0 {
		if _, e := rf.WriteBuf(buf); e != nil {
			fmt.Printf("[log] write rotate file error: %s with data: %s\n", e, buf.String())
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
