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

	"{{.}}/config"
	"{{.}}/discovery"
	"{{.}}/server/xgrpc"
	"{{.}}/server/xhttp"
	"{{.}}/service"
)

func main() {
	//start the whole business service
	//discovery register will in here
	service.StartService()
	//start low level net service
	ch := make(chan os.Signal, 1)
	wg := &sync.WaitGroup{}
	//grpc server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xgrpc.StartGrpcServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	//http server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xhttp.StartHttpServer()
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
		wg.Done()
	}()
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-ch
	//stop watching config hot update
	config.Close()
	//stop the whole business service
	//discovery unregister will in here
	service.StopService()
	//stop discover cluster info
	discovery.Close()
	//stop low level net service
	//grpc server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xgrpc.StopGrpcServer()
		wg.Done()
	}()
	//http server,if don't need,please comment this
	wg.Add(1)
	go func() {
		xhttp.StopHttpServer()
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
