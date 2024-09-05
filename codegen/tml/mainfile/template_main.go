package mainfile

import (
	"os"
	"text/template"
)

const txt = `package main

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"{{.}}/config"
	"{{.}}/dao"
	"{{.}}/server/xcrpc"
	"{{.}}/server/xgrpc"
	"{{.}}/server/xweb"
	"{{.}}/service"

	publicmids "github.com/chenjie199234/Corelib/mids"
	_ "github.com/chenjie199234/Corelib/monitor"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/redis/go-redis/v9"
	_ "go.mongodb.org/mongo-driver/mongo"
)

func main() {
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
		publicmids.UpdateTokenConfig(ac.TokenSecret, ac.SessionTokenExpire.StdDuration())
		publicmids.UpdateSessionConfig(ac.SessionTokenExpire.StdDuration())
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
	wg.Wait()
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
