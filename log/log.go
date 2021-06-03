package log

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
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
var logcaller bool   //true(default),false
//this is only working when log will write to file
var logdir string //default: ./log
//this is only working when log will write to file
var logname string //default: log
//this is only working when log will write to file
var logrotatecap uint64 //0 don't rotate by file size,unit is M
//this is only working when log will write to file
var logrotatecycle uint64 //0 don't rotate by time,1 every hour,2 every day,3 every week,4 every month
//this is only working when log will write to file
var logkeepdays uint64 //0 don't delete old log file,unit day

func getenv() {
	var e error
	if os.Getenv("LOG_TARGET") == "" {
		logtarget = "std"
	} else {
		temp := strings.ToLower(os.Getenv("LOG_TARGET"))
		if temp != "std" && temp != "file" && temp != "both" {
			panic("[log] os env LOG_TARGET error,must in [std(default when not set),file,both] or not set")
		}
		logtarget = temp
	}
	if os.Getenv("LOG_LEVEL") == "" {
		loglevel = "info"
	} else {
		temp := strings.ToLower(os.Getenv("LOG_LEVEL"))
		if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" {
			panic("[log] os env LOG_LEVEL error,must in [debug,info(default when not set),warning,error] or not set")
		}
		loglevel = temp
	}
	if os.Getenv("LOG_CALLER") == "" {
		logcaller = true
	} else {
		temp := strings.ToLower(os.Getenv("LOG_CALLER"))
		if temp != "true" && temp != "false" {
			panic("[log] os env LOG_CALLER error,must in [true(default when not set),false] or not set")
		}
		logcaller = temp == "true"
	}
	if logtarget == "file" || logtarget == "both" {
		if os.Getenv("LOG_DIR") == "" {
			logdir = "./log"
		} else {
			logdir = os.Getenv("LOG_DIR")
		}
		if os.Getenv("LOG_NAME") == "" {
			logname = "log"
		} else {
			logname = os.Getenv("LOG_NAME")
		}
		if os.Getenv("LOG_ROTATE_CAP") == "" {
			logrotatecap = 0
		} else if logrotatecap, e = strconv.ParseUint(os.Getenv("LOG_ROTATE_CAP"), 10, 64); e != nil {
			panic("[log.getenv] LOG_ROTATE_CAP must be integer,unit is M")
		}
		if os.Getenv("LOG_ROTATE_CYCLE") == "" {
			logrotatecycle = 0
		} else if logrotatecycle, e = strconv.ParseUint(os.Getenv("LOG_ROTATE_CYCLE"), 10, 64); e != nil {
			panic("[log.getenv] LOG_ROTATE_CYCLE must be integer in [0-don't rotate by time(default when not set),1-every hour,2-every day,3-every week,4-every month] or not set")
		} else if logrotatecycle != 0 && logrotatecycle != 1 && logrotatecycle != 2 && logrotatecycle != 3 && logrotatecycle != 4 {
			panic("[log.getenv] LOG_ROTATE_CYCLE must be integer in [0-don't rotate by time(default when not set),1-every hour,2-every day,3-every week,4-every month] or not set")
		}
		if os.Getenv("LOG_KEEP_DAYS") == "" {
			logkeepdays = 0
		} else if logkeepdays, e = strconv.ParseUint(os.Getenv("LOG_KEEP_DAYS"), 10, 64); e != nil {
			panic("[log.getenv] LOG_KEEP_DAYS must be integer,unit is day")
		}
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
		instance.rf, e = rotatefile.NewRotateFile(&rotatefile.Config{
			Path:        logdir,
			Name:        logname,
			RotateCap:   uint(logrotatecap),
			RotateCycle: uint(logrotatecycle),
			KeepDays:    uint(logkeepdays),
		})
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
func Must(datas ...interface{}) {
	buf := bufpool.GetBuffer()
	buf.Append("[MST] ")
	write(buf, datas...)
}
func write(buf *bufpool.Buffer, datas ...interface{}) {
	buf.Append(time.Now())
	if logcaller {
		_, file, line, _ := runtime.Caller(2)
		buf.Append(" ")
		buf.Append(file)
		buf.Append(":")
		buf.Append(line)
	}
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
