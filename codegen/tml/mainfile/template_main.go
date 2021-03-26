package mainfile

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"{{.}}/config"
	"{{.}}/server/xrpc"
	"{{.}}/server/xweb"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/log"
	discoverysdk "github.com/chenjie199234/Discovery/sdk"
)

func main() {
	defer config.Close()
	//start the whole business service
	if e := service.StartService(); e != nil {
		log.Error(e)
		return
	}
	//start low level net service
	ch := make(chan os.Signal, 1)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		xrpc.StartRpcServer()
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
	//try to register self to the discovery server
	stop := make(chan struct{})
	go func() {
		//delay 200ms to register self,if error happened in this 200ms,this server will not be registered
		tmer := time.NewTimer(time.Millisecond * 200)
		select {
		case <-tmer.C:
			webc := config.GetWebServerConfig()
			if webc != nil && len(webc.CertKey) > 0 {
				discoverysdk.RegisterSelf(9000, 8000, "https", nil)
			} else {
				discoverysdk.RegisterSelf(9000, 8000, "http", nil)
			}
		case <-stop:
		}
	}()
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ch
	close(stop)
	//stop the whole business service
	service.StopService()
	//stop low level net service
	//grpc server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xrpc.StopRpcServer()
		wg.Done()
	}()
	//http server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xweb.StopWebServer()
		wg.Done()
	}()
	wg.Wait()
}`

const path = "./"
const name = "main.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("main").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template for %s error:%s", path+name, e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+name, e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+name, e))
	}
}
