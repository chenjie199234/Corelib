package log

import (
	"context"
	"fmt"
	"io"
	glog "log"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/ctime"
)

var logtrace bool
var target io.Writer
var level slog.Level
var rf *rotatefile.RotateFile

var sloger *slog.Logger

func init() {
	if str := os.Getenv("LOG_TRACE"); str != "" && str != "<LOG_TRACE>" && str != "0" && str != "1" {
		panic("[log] os env LOG_TRACE error,must in [0,1]")
	} else {
		logtrace = str == "1"
	}
	if str := strings.ToLower(os.Getenv("LOG_TARGET")); str != "std" && str != "file" && str != "" && str != "<LOG_TARGET>" {
		panic("[log] os env LOG_TARGET error,must in [std(default),file]")
	} else {
		switch str {
		case "<LOG_TARGET>":
			//default std
			fallthrough
		case "":
			//default std
			fallthrough
		case "std":
			target = os.Stdout
		case "file":
			var e error
			rf, e = rotatefile.NewRotateFile("./log", "log")
			if e != nil {
				panic("[log] create rotate file failed: " + e.Error())
			}
			target = rf
		}
	}
	if str := strings.ToLower(os.Getenv("LOG_LEVEL")); str != "debug" && str != "info" && str != "warn" && str != "error" && str != "" && str != "<LOG_LEVEL>" {
		panic("[log] os env LOG_LEVEL error,must in [debug,info(default),warn,error]")
	} else {
		switch str {
		case "debug":
			level = slog.LevelDebug
		case "<LOG_LEVEL>":
			//default info
			fallthrough
		case "":
			//default info
			fallthrough
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	replace := func(groups []string, attr slog.Attr) slog.Attr {
		if len(groups) == 0 && attr.Key == "msg" {
			msg, ok := attr.Value.Any().(string)
			if ok && msg == "" {
				return slog.Attr{}
			}
			return attr
		}
		if len(groups) == 0 && attr.Key == "function" {
			return slog.Attr{}
		}
		if len(groups) == 0 && attr.Key == slog.SourceKey {
			s := attr.Value.Any().(*slog.Source)
			if index := strings.Index(s.File, "corelib@v"); index != -1 {
				s.File = s.File[index:]
			} else if index = strings.Index(s.File, "Corelib@v"); index != -1 {
				s.File = s.File[index:]
			}
		}
		return attr
	}
	sloger = slog.New(slog.NewJSONHandler(target, &slog.HandlerOptions{AddSource: true, Level: level, ReplaceAttr: replace}).WithGroup("attrs"))
	trace.SetSloger(sloger)
}

// get slog.Logger,usually this is not used,Debug,Info,Warn,Error is prefered
func GetSloger() *slog.Logger {
	return sloger
}

// get the log.Logger,usually this is not used,Debug,Info,Warn,Error is prefered
func GetLloger() *glog.Logger {
	return slog.NewLogLogger(slog.NewJSONHandler(target, &slog.HandlerOptions{AddSource: true, Level: level}), level)
}

func Debug(ctx context.Context, msg string, attrs ...any) {
	innerlog(ctx, slog.LevelDebug, msg, attrs...)
}
func Info(ctx context.Context, msg string, attrs ...any) {
	innerlog(ctx, slog.LevelInfo, msg, attrs...)
}
func Warn(ctx context.Context, msg string, attrs ...any) {
	innerlog(ctx, slog.LevelWarn, msg, attrs...)
}
func Error(ctx context.Context, msg string, attrs ...any) {
	innerlog(ctx, slog.LevelError, msg, attrs...)
}
func innerlog(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !sloger.Enabled(ctx, level) {
		return
	}
	if logtrace {
		if span := trace.SpanFromContext(ctx); span != nil {
			attrs = append(attrs, slog.String("traceid", span.GetSelfSpanData().GetTid().String()))
		}
	}
	var pcs [1]uintptr
	//skip runtime.Callers ,this function ,and innerlog's caller
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(attrs...)
	sloger.Handler().Handle(ctx, r)
}
func String(key, value string) slog.Attr {
	return slog.String(key, value)
}

func Int64(key string, value int64) slog.Attr {
	return slog.Int64(key, value)
}

func Int(key string, value int) slog.Attr {
	return slog.Int(key, value)
}

func Uint64(key string, value uint64) slog.Attr {
	return slog.Uint64(key, value)
}

func Float64(key string, value float64) slog.Attr {
	return slog.Float64(key, value)
}

func Bool(key string, value bool) slog.Attr {
	return slog.Bool(key, value)
}

func Time(key string, value time.Time) slog.Attr {
	return slog.Time(key, value)
}

func Duration(key string, value time.Duration) slog.Attr {
	return slog.Duration(key, value)
}

func CDuration(key string, value ctime.Duration) slog.Attr {
	return slog.String(key, value.String())
}

func CError(value error) slog.Attr {
	e := cerror.ConvertStdError(value)
	if e == nil {
		return slog.Any("error", nil)
	}
	return Group("error", slog.Int64("code", int64(e.Code)), slog.String("msg", e.Msg))
}

func Group(key string, attrs ...slog.Attr) slog.Attr {
	return slog.Attr{Key: key, Value: slog.GroupValue(attrs...)}
}

func Any(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

func LogFileSize() int64 {
	return rf.GetCurFileLen()
}
func RotateLogFile() {
	if rf != nil {
		if e := rf.RotateNow(); e != nil {
			fmt.Printf("[log] rotate log file error:%s\n", e)
		}
	}
}
func CleanLogFile(lastModTimestampBeforeThisNS int64) {
	if rf != nil {
		if e := rf.CleanNow(lastModTimestampBeforeThisNS); e != nil {
			fmt.Printf("[log] clean log file before timestamp:%dns error:%s\n", lastModTimestampBeforeThisNS, e)
		}
	}
}
func Close() {
	if rf != nil {
		rf.Close()
	}
}
