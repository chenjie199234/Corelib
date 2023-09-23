package log

import (
	"context"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/util/host"
)

type ROLE string

const (
	CLIENT ROLE = "client"
	SERVER ROLE = "server"
)

type tracekey struct{}

func InitTrace(ctx context.Context, traceid, app, ip, method, path string, deep int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	tmp, ok := ctx.Value(tracekey{}).(map[string]string)
	if !ok || tmp["Traceid"] == "" {
		if app == "" || ip == "" || method == "" || path == "" {
			panic("[trace] init error: missing params")
		}
		if traceid == "" {
			deep = 0
			traceid = maketraceid()
		}
		deep++
		return context.WithValue(ctx, tracekey{}, map[string]string{"Traceid": traceid, "Deep": strconv.Itoa(deep), "App": app, "Ip": ip, "Method": method, "Path": path})
	}
	return ctx
}

func GetTrace(ctx context.Context) (traceid, curapp, curip, curmethod, curpath string, curdeep int) {
	if ctx == nil {
		return
	}
	tracedata, ok := ctx.Value(tracekey{}).(map[string]string)
	if !ok || tracedata["Traceid"] == "" {
		return
	}
	traceid = tracedata["Traceid"]
	curapp = tracedata["App"]
	curip = tracedata["Ip"]
	curmethod = tracedata["Method"]
	curpath = tracedata["Path"]
	curdeep, _ = strconv.Atoi(tracedata["Deep"])
	return
}

// this will overwrite dst's tracedata
func CopyTrace(src, dst context.Context) (context.Context, bool) {
	if dst == nil {
		dst = context.Background()
	}
	if src == nil {
		return dst, false
	}
	tracedata, ok := src.Value(tracekey{}).(map[string]string)
	if ok && tracedata["Traceid"] != "" {
		return context.WithValue(dst, tracekey{}, tracedata), true
	}
	return dst, false
}

func maketraceid() string {
	nowstr := strconv.FormatInt(time.Now().UnixNano(), 10)
	ranstr := strconv.FormatInt(rand.Int63(), 10)
	return nowstr + "_" + ranstr
}

func Trace(ctx context.Context, role ROLE, toapp, toip, tomethod, topath string, start, end *time.Time, e error) {
	if !trace {
		return
	}
	traceid, fromapp, fromip, frommethod, frompath, deep := GetTrace(ctx)
	if traceid == "" {
		return
	}
	attrs := make([]any, 0)
	attrs = append(attrs, Group("trace", slog.String("id", traceid)), slog.Int("deep", deep))
	attrs = append(attrs, slog.Int64("start", start.UnixNano()))
	attrs = append(attrs, slog.Int64("end", end.UnixNano()))
	attrs = append(attrs, slog.String("host_name", host.Hostname))
	attrs = append(attrs, slog.String("role", string(role)))
	attrs = append(attrs, slog.String("from_app", fromapp))
	attrs = append(attrs, slog.String("from_ip", fromip))
	attrs = append(attrs, slog.String("from_method", frommethod))
	attrs = append(attrs, slog.String("from_path", frompath))
	attrs = append(attrs, slog.String("to_app", toapp))
	attrs = append(attrs, slog.String("to_ip", toip))
	attrs = append(attrs, slog.String("to_method", tomethod))
	attrs = append(attrs, slog.String("to_path", topath))
	attrs = append(attrs, CError(e))
	innerlog(ctx, slog.LevelInfo, "trace", attrs...)
}
