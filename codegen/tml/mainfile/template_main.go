package mainfile

import (
	"os"
	"text/template"
)

const txt = `package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"net/http"
	_ "net/http/pprof"

	"{{.}}/config"
	"{{.}}/dao"
	_ "{{.}}/model"
	"{{.}}/server/xcrpc"
	"{{.}}/server/xgrpc"
	"{{.}}/server/xraw"
	"{{.}}/server/xweb"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/cotel"
	publicmids "github.com/chenjie199234/Corelib/mids"
	_ "github.com/chenjie199234/Corelib/monitor"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/redis/go-redis/v9"
	_ "go.mongodb.org/mongo-driver/v2/mongo"
)

type LogHandler struct {
	slog.Handler
}

func (l *LogHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.NumAttrs() > 0 {
		attrs := make([]slog.Attr, 0, record.NumAttrs())
		record.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a)
			return true
		})
		if record.Message == "trace" {
			record.PC = 0
		}
		record = slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
		record.AddAttrs(slog.Attr{
			Key:   "msg_kvs",
			Value: slog.GroupValue(attrs...),
		})
	}
	if traceid := cotel.TraceIDFromContext(ctx); traceid != "" {
		record.AddAttrs(slog.String("traceid", traceid))
	}
	return l.Handler.Handle(ctx, record)
}

func main() {
	slog.SetDefault(slog.New(&LogHandler{
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
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
			},
		}),
	}))
	config.Init(func(ac *config.AppConfig) {
		//this is a notice callback every time appconfig changes
		//this function works in sync mode
		//don't write block logic inside this
		dao.UpdateAppConfig(ac)
		xcrpc.UpdateHandlerTimeout(ac.HandlerTimeout)
		xgrpc.UpdateHandlerTimeout(ac.HandlerTimeout)
		xweb.UpdateHandlerTimeout(ac.HandlerTimeout)
		xweb.UpdateWebPathRewrite(ac.WebPathRewrite)
		publicmids.UpdateRateConfig(ac.HandlerRate)
		publicmids.UpdateTokenConfig(ac.TokenSecret)
		publicmids.UpdateAccessConfig(ac.Accesses)
	})
	if rateredis := config.GetRedis("rate_redis"); rateredis != nil {
		publicmids.UpdateRateRedisInstance(rateredis)
	} else {
		slog.WarnContext(nil, "[main] rate redis missing,all rate check will be failed")
	}
	if sessionredis := config.GetRedis("session_redis"); sessionredis != nil {
		publicmids.UpdateSessionRedisInstance(sessionredis)
	} else {
		slog.WarnContext(nil, "[main] session redis missing,all session event will be failed")
	}
	//start the whole business service
	if e := service.StartService(); e != nil {
		slog.ErrorContext(nil, "[main] start service failed", slog.String("error",e.Error()))
		return
	}
	//start low level net service
	ch := make(chan os.Signal, 1)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		xcrpc.StartCrpcServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xweb.StartWebServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xgrpc.StartCGrpcServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xraw.StartRawServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	pprofserver := &http.Server{addr:":6060"}
	wg.Add(1)
	go func(){
		pprofserver.ListenAndServe()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ch
	//stop the whole business service
	service.StopService()
	//stop low level net service
	wg.Add(1)
	go func() {
		xcrpc.StopCrpcServer(false)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xweb.StopWebServer(false)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xgrpc.StopCGrpcServer(false)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		xraw.StopRawServer()
		wg.Done()
	}()
	wg.Add(1)
	go func(){
	  pprofserver.Shutdown(context.Background())
	  wg.Done()
	}()
	wg.Wait()
	cotel.Stop()
}`

func CreatePathAndFile(packagename string) {
	maintemplate, e := template.New("./main.go").Parse(txt)
	if e != nil {
		panic("parse ./main.go template error: " + e.Error())
	}
	file, e := os.OpenFile("./main.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./main.go error: " + e.Error())
	}
	if e := maintemplate.Execute(file, packagename); e != nil {
		panic("write ./main.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./main.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./main.go error: " + e.Error())
	}
}
