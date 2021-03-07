package log

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

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
	inited int64
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

//func getflag() {
//        var target = flag.String("logtarget", "", strings.Join([]string{
//                "std,file,both",
//                "this will overwrite the setting from os's env LOG_TARGET"}, "\n"))
//        var level = flag.String("loglevel", "", strings.Join([]string{
//                "Debug,Info,Warning,Error",
//                "this will overwrite the setting from os's env LOG_LEVEL"}, "\n"))
//        var caller = flag.String("logcaller", "", strings.Join([]string{
//                "true,false",
//                "this will overwrite the setting from os's env LOG_CALLER"}, "\n"))
//        var name = flag.String("logname", "", strings.Join([]string{
//                "this will only useful when log write to file",
//                "this will overwrite the setting from os's env LOG_NAME"}, "\n"))
//        var dir = flag.String("logdir", "", strings.Join([]string{
//                "this will only useful when log write to file",
//                "this will overwrite the setting from os's env LOG_DIR"}, "\n"))
//        var cap = flag.String("logrotatecap", "", strings.Join([]string{
//                "0 don't rotate by file size,unit is M",
//                "this will only useful when log write to file",
//                "this will overwrite the setting from os's env LOG_ROTATE_CAP"}, "\n"))
//        var cycle = flag.String("logrotatecycle", "", strings.Join([]string{
//                "0-don't rotate by time,1-every hour,2-every day,3-every week,4-every month",
//                "this will only useful when log write to file",
//                "this will overwrite the setting from os's env LOG_ROTATE_CYCLE"}, "\n"))
//        var keep = flag.String("logkeepdays", "", strings.Join([]string{
//                "0 don't delete old log file,unit day",
//                "this will only useful when log write to file",
//                "this will overwrite the setting from os's env LOG_KEEP_DAYS"}, "\n"))

//        if target != nil && *target != "" {
//                temp := strings.ToLower(*target)
//                if temp != "std" && temp != "file" && temp != "both" {
//                        panic("[log.getflag] input flag for logtarget error,must in [std,file,both]")
//                }
//                logtarget = temp
//        }
//        if level != nil && *level != "" {
//                temp := strings.ToLower(*level)
//                if temp != "debug" && temp != "info" && temp != "warning" && temp != "error" {
//                        panic("[log.getflag] input flag for loglevel error,must in [Debug,Info,Warning,Error]")
//                }
//                loglevel = temp
//        }
//        if level != nil && *caller != "" {
//                temp := strings.ToLower(*caller)
//                if temp != "true" && temp != "false" {
//                        panic("[log.getflag] input flag for logcaller error,must in [true,false]")
//                }
//                logcaller = temp == "true"
//        }
//        if logtarget == "file" || logtarget == "both" {
//                if name != nil && *name != "" {
//                        logname = *name
//                }
//                if dir != nil && *dir != "" {
//                        logdir = *dir
//                }
//                if cap != nil && *cap != "" {
//                        temp, e := strconv.ParseUint(*cap, 10, 64)
//                        if e != nil {
//                                panic("[log.getflag] input flag for logrotatecap must be integer,unit is M")
//                        }
//                        logrotatecap = temp
//                }
//                if cycle != nil && *cycle != "" {
//                        temp, e := strconv.ParseUint(*cycle, 10, 64)
//                        if e != nil {
//                                panic("[log.getflag] input flag for logrotatecycle must be integer in [0-don't rotate by time,1-every hour,2-every day,3-every week,4-every month]")
//                        }
//                        if temp != 0 && temp != 1 && temp != 2 && temp != 3 && temp != 4 {
//                                panic("[log.getflag] input flag for logrotatecycle must be integer in [0-don't rotate by time,1-every hour,2-every day,3-every week,4-every month]")
//                        }
//                        logrotatecycle = temp
//                }
//                if keep != nil && *keep != "" {
//                        temp, e := strconv.ParseUint(*keep, 10, 64)
//                        if e != nil {
//                                panic("[log.getflag] input flag for logkeepdays must be integer,unit is day")
//                        }
//                        logkeepdays = temp
//                }
//        }
//}

func init() {
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&instance)), nil, unsafe.Pointer(&log{inited: 1})) {
		return
	}
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
			Ext:         "log",
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
	if atomic.SwapInt64(&(instance.inited), 0) == 0 {
		return
	}
	if instance.target&targetFile > 0 && instance.rf != nil {
		instance.rf.Close()
	}
}
