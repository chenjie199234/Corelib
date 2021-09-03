package log

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
)

var logtarget int //1-std(default),2-file,3-both
var loglevel int  //0-debug,1-info(default),2-warning,3-error
var rf *rotatefile.RotateFile

func getenv() {
	if os.Getenv("LOG_TARGET") == "" {
		//default std
		logtarget = 1
	} else {
		temp := strings.ToLower(os.Getenv("LOG_TARGET"))
		if temp != "std" && temp != "file" && temp != "both" {
			panic("[log] os env LOG_TARGET error,must in [std(default),file,both]")
		}
		switch temp {
		case "std":
			loglevel = 1
		case "file":
			loglevel = 2
		case "both":
			loglevel = 3
		}
	}
	if os.Getenv("LOG_LEVEL") == "" {
		//default info
		loglevel = 1
	} else {
		temp := strings.ToLower(os.Getenv("LOG_LEVEL"))
		if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" {
			panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warning,error]")
		}
		switch temp {
		case "debug":
			loglevel = 0
		case "info":
			loglevel = 1
		case "warning":
			loglevel = 2
		case "error":
			loglevel = 3
		}
	}
}

func init() {
	getenv()
	if logtarget&2 > 0 {
		var e error
		rf, e = rotatefile.NewRotateFile("./log", "log")
		if e != nil {
			panic("[log]create rotate log file error:" + e.Error())
		}
	}
}
func Debug(datas ...interface{}) {
	if loglevel > 0 {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[DBG] ")
	write(buf, datas...)
}
func Info(datas ...interface{}) {
	if loglevel > 1 {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[INF] ")
	write(buf, datas...)
}
func Warning(datas ...interface{}) {
	if loglevel > 2 {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[WRN] ")
	write(buf, datas...)
}
func Error(datas ...interface{}) {
	buf := bufpool.GetBuffer()
	buf.Append("[ERR] ")
	write(buf, datas...)
}
func write(buf *bufpool.Buffer, datas ...interface{}) {
	buf.Append(time.Now())
	_, file, line, _ := runtime.Caller(2)
	buf.Append(" ")
	buf.Append(file)
	buf.Append(":")
	buf.Append(line)
	for _, data := range datas {
		buf.Append(" ")
		buf.Append(data)
	}
	buf.Append("\n")
	if logtarget&1 > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if logtarget&2 > 0 {
		if _, e := rf.WriteBuf(buf); e != nil {
			fmt.Printf("[log] write rotate file error: %s with data: %s\n", e, buf.String())
			bufpool.PutBuffer(buf)
		}
	} else {
		bufpool.PutBuffer(buf)
	}
}
func RotateLogFile() {
	if logtarget&2 > 0 && rf != nil {
		if e := rf.RotateNow(); e != nil {
			fmt.Printf("[log] rotate log file error:%s\n", e)
		}
	}
}
func CleanLogFile(lastModTimestampBeforeThisNS int64) {
	if logtarget&2 > 0 && rf != nil {
		if e := rf.CleanNow(lastModTimestampBeforeThisNS); e != nil {
			fmt.Printf("[log] clean log file before:%dns error:%s\n", lastModTimestampBeforeThisNS, e)
		}
	}
}
func Close() {
	if logtarget&2 > 0 && rf != nil {
		rf.Close()
	}
}
