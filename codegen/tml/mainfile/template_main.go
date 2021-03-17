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

	"{{.}}/api"
	"{{.}}/config"
	"{{.}}/server/xrpc"
	"{{.}}/server/xweb"
	"{{.}}/service"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/log"
)

func main() {
	//stop watching config hot update
	defer config.Close()
	discoveryserververifydata := os.Getenv("DISCOVERY_SERVER_VERIFY_DATA")
	if discoveryserververifydata != "" {
		if e := discovery.NewDiscoveryClient(nil, api.Group, api.Name, []byte(discoveryserververifydata), nil); e != nil {
			log.Error(e)
			return
		}
	}
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
	stop := make(chan struct{}, 1)
	go func() {
		//delay 200ms to register self,if error happened in this 200ms,this server will not be registered
		tmer := time.NewTimer(time.Millisecond * 200)
		select {
		case <-tmer.C:
			rpcc := config.GetRpcConfig()
			webc := config.GetWebConfig()
			regmsg := &discovery.RegMsg{}
			if webc != nil {
				if webc.WebKeyFile != "" && webc.WebCertFile != "" {
					regmsg.WebScheme = "https"
				} else {
					regmsg.WebScheme = "http"
				}
				regmsg.WebPort = 8000
			}
			if rpcc != nil {
				regmsg.RpcPort = 9000
			}
			discovery.RegisterSelf(regmsg)
		case <-stop:
		}
	}()
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ch
	stop <- struct{}{}
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
