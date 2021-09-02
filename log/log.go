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

const (
	debugLevel = iota
	infoLevel
	warningLevel
	errorLevel
)
const (
	targetStd = iota + 1
	targetFile
	targetBoth
)

type log struct {
	level  int
	target int
	rf     *rotatefile.RotateFile
}

var instance *log

var logtarget string //std(default),file,both
var loglevel string  //debug,info(default),warning,error

func getenv() {
	if os.Getenv("LOG_TARGET") == "" {
		logtarget = "std"
	} else {
		temp := strings.ToLower(os.Getenv("LOG_TARGET"))
		if temp != "std" && temp != "file" && temp != "both" {
			panic("[log] os env LOG_TARGET error,must in [std(default),file,both]")
		}
		logtarget = temp
	}
	if os.Getenv("LOG_LEVEL") == "" {
		loglevel = "info"
	} else {
		temp := strings.ToLower(os.Getenv("LOG_LEVEL"))
		if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" {
			panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warning,error]")
		}
		loglevel = temp
	}
}

func init() {
	instance = &log{}
	getenv()
	switch logtarget {
	case "std":
		instance.target = targetStd
	case "file":
		instance.target = targetFile
	case "both":
		instance.target = targetBoth
	}
	switch loglevel {
	case "debug":
		instance.level = debugLevel
	case "info":
		instance.level = infoLevel
	case "warning":
		instance.level = warningLevel
	case "error":
		instance.level = errorLevel
	}
	if logtarget == "file" || logtarget == "both" {
		var e error
		instance.rf, e = rotatefile.NewRotateFile("./log", "log")
		if e != nil {
			panic("[log]create rotate log file error:" + e.Error())
		}
	}
}
func Debug(datas ...interface{}) {
	if instance.level > debugLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[DBG] ")
	write(buf, datas...)
}
func Info(datas ...interface{}) {
	if instance.level > infoLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[INF] ")
	write(buf, datas...)
}
func Warning(datas ...interface{}) {
	if instance.level > warningLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[WRN] ")
	write(buf, datas...)
}
func Error(datas ...interface{}) {
	if instance.level > errorLevel {
		return
	}
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
	if instance.target&targetStd > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if instance.target&targetFile > 0 {
		if _, e := instance.rf.WriteBuf(buf); e != nil {
			fmt.Printf("[log] write rotate file error: %s with data: %s\n", e, buf.String())
			bufpool.PutBuffer(buf)
		}
	} else {
		bufpool.PutBuffer(buf)
	}
}
func Close() {
	if instance.target&targetFile > 0 && instance.rf != nil {
		instance.rf.Close()
	}
}
