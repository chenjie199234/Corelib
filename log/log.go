package log

import (
	"flag"
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
	target int
	level  int
	rf     *rotatefile.RotateFile
}

var instance *log

var logname = flag.String("logname", "", "log file's name")
var logdir = flag.String("logdir", "./", "log file's dir")
var logcap = flag.Uint64("logcap", 0, "max size of each log file\npass will rotate\nunit M")
var logcycle = flag.String("logcycle", "off", "rotate log file every\noff,hour,day,month")
var logtarget = flag.String("logtarget", "std", "where to output log\nstd,file,both")
var loglevel = flag.String("loglevel", "Info", "min level can be logged\nDebug,Info,Warning,Error")
var logkeep = flag.Uint64("logkeepday", 0, "log file will keep how many days")
var logfileline = flag.Bool("logfileline", false, "add log caller's file and line number")

func init() {
	instance = &log{}
	switch strings.ToLower(*logtarget) {
	case "":
		fallthrough
	case "std":
		instance.target = targetStd
	case "file":
		instance.target = targetFile
	case "both":
		instance.target = targetBoth
	default:
		panic("[mlog]target unknown,supported:std,file,both.default is std")
	}
	switch strings.ToLower(*loglevel) {
	case "debug":
		instance.level = debugLevel
	case "":
		fallthrough
	case "info":
		instance.level = infoLevel
	case "warning":
		instance.level = warningLevel
	case "error":
		instance.level = errorLevel
	default:
		panic("[mlog]log level unknown,supported:debug,info,warning,error.default is info")
	}
	if instance.target&targetFile > 0 {
		var e error
		switch strings.ToLower(*logcycle) {
		case "off":
			instance.rf, e = rotatefile.NewRotateFile(*logdir, *logname, "log", rotatefile.RotateCap(*logcap), rotatefile.RotateOff, rotatefile.KeepDays(*logkeep))
		case "hour":
			instance.rf, e = rotatefile.NewRotateFile(*logdir, *logname, "log", rotatefile.RotateCap(*logcap), rotatefile.RotateHour, rotatefile.KeepDays(*logkeep))
		case "day":
			instance.rf, e = rotatefile.NewRotateFile(*logdir, *logname, "log", rotatefile.RotateCap(*logcap), rotatefile.RotateDay, rotatefile.KeepDays(*logkeep))
		case "month":
			instance.rf, e = rotatefile.NewRotateFile(*logdir, *logname, "log", rotatefile.RotateCap(*logcap), rotatefile.RotateMonth, rotatefile.KeepDays(*logkeep))
		default:
			panic("[mlog]log cycle unknown,supported:hour,day,month.default is no cycle")
		}
		if e != nil {
			panic("[mlog]create rotate log file error:" + e.Error())
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
	if *logfileline {
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
			fmt.Println("[mlog] write rotate file error: " + e.Error())
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
