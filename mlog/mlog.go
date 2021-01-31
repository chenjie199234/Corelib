package mlog

import (
	"flag"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
)

const (
	debugLevel = iota
	infoLevel
	warningLevel
	errorLevel
	panicLevel
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
	kvpool *sync.Pool
}
type kv struct {
	key   string
	value interface{}
}

var instance *log

var logname = flag.String("logname", "", "log file's name")
var logdir = flag.String("logdir", "./", "log file's dir")
var logcap = flag.Uint64("logcap", 0, "max size of each log file\npass will rotate\nunit M")
var logcycle = flag.String("logcycle", "off", "rotate log file every\noff,hour,day,month")
var logtarget = flag.String("logtarget", "std", "where to output log\nstd,file,both")
var loglevel = flag.String("loglevel", "Info", "min level can be logged\nDebug,Info,Warning,Error,Panic")

func init() {
	flag.Parse()
	instance = &log{
		kvpool: &sync.Pool{},
	}
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
	case "panic":
		instance.level = panicLevel
	default:
		panic("[mlog]log level unknown,supported:debug,info,warning,error,panic.default is debug")
	}
	if instance.target&targetFile > 0 {
		var e error
		switch strings.ToLower(*logcycle) {
		case "off":
			instance.rf, e = rotatefile.NewRotateFile(*logdir+"/", *logname, rotatefile.RotateCap(*logcap), rotatefile.RotateOff)
		case "hour":
			instance.rf, e = rotatefile.NewRotateFile(*logdir+"/", *logname, rotatefile.RotateCap(*logcap), rotatefile.RotateHour)
		case "day":
			instance.rf, e = rotatefile.NewRotateFile(*logdir+"/", *logname, rotatefile.RotateCap(*logcap), rotatefile.RotateDay)
		case "month":
			instance.rf, e = rotatefile.NewRotateFile(*logdir+"/", *logname, rotatefile.RotateCap(*logcap), rotatefile.RotateMonth)
		default:
			panic("[mlog]log cycle unknown,supported:hour,day,month.default is no cycle")
		}
		if e != nil {
			panic("[mlog]create rotate log file error:" + e.Error())
		}
	}
}
func KV(key string, value interface{}) *kv {
	temp, ok := instance.kvpool.Get().(*kv)
	if !ok {
		return &kv{key: key, value: value}
	}
	temp.key = key
	temp.value = value
	return temp
}
func Debug(msg string, datas ...kv) {
	if instance.level > debugLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[DBG] ")
	buf.Append(time.Now())
	buf.Append(" msg:")
	buf.Append(msg)
	write(buf, datas...)
}
func Info(msg string, datas ...kv) {
	if instance.level > infoLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[INF] ")
	buf.Append(time.Now())
	buf.Append(" msg:")
	buf.Append(msg)
	write(buf, datas...)
}
func Warning(msg string, datas ...kv) {
	if instance.level > warningLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[WRN] ")
	buf.Append(time.Now())
	buf.Append(" msg:")
	buf.Append(msg)
	write(buf, datas...)
}
func Error(msg string, datas ...kv) {
	if instance.level > errorLevel {
		return
	}
	buf := bufpool.GetBuffer()
	buf.Append("[ERR] ")
	buf.Append(time.Now())
	buf.Append(" msg:")
	buf.Append(msg)
	write(buf, datas...)
}
func write(buf *bufpool.Buffer, kvs ...kv) {
	for _, temp := range kvs {
		buf.Append(" ")
		buf.Append(temp.key)
		buf.Append(":")
		buf.Append(temp.value)
		instance.kvpool.Put(temp)
	}
	buf.Append("\n")
	if instance.target&targetStd > 0 {
		os.Stderr.Write(buf.Bytes())
	}
	if instance.target&targetFile > 0 {
		instance.rf.WriteBuf(buf)
	}
	bufpool.PutBuffer(buf)
}
func Close(wait bool) {
	if instance.target&targetFile > 0 && instance.rf != nil {
		instance.rf.Close(wait)
	}
}
