package config

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

const text = `package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	//make sure source package is the first init
	_ "{{.}}/source"

	"github.com/fsnotify/fsnotify"
	"github.com/chenjie199234/Corelib/log"
)

//AppConfig can hot update
//this is the config used for this app
type AppConfig struct {
	//add your config here
}

//AC -
var AC *AppConfig

var watcher *fsnotify.Watcher
var closech chan struct{}

func init() {
	data, e := ioutil.ReadFile("AppConfig.json")
	if e != nil {
		log.Fatalf("[AppConfig]read config file error:%s", e)
	}
	AC = &AppConfig{}
	if e = json.Unmarshal(data, AC); e != nil {
		log.Fatalf("[AppConfig]config data format error:%s", e)
	}
	watcher, e = fsnotify.NewWatcher()
	if e != nil {
		log.Fatalf("[AppConfig]create watcher for hot update error:%s", e)
	}
	if e = watcher.Add("./"); e != nil {
		log.Fatalf("[AppConfig]create watcher for hot update error:%s", e)
	}
	closech = make(chan struct{})
	go watch()
}
func watch() {
	defer close(closech)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != "AppConfig.json" || (event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Write == 0) {
				continue
			}
			data, e := ioutil.ReadFile("AppConfig.json")
			if e != nil {
				log.Errorf("[AppConfig]hot update read config file error:%s", e)
				continue
			}
			c := &AppConfig{}
			if e = json.Unmarshal(data, c); e != nil {
				log.Errorf("[AppConfig]hot update config data format error:%s", e)
				continue
			}
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&AC)), unsafe.Pointer(c))
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Errorf("[AppConfig]watcher error,%s", err)
		}
	}
}
func Close() {
	watcher.Close()
	<-closech
}`
const path = "./config/"
const name = "config.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("config").Parse(strings.ReplaceAll(text, "$", "`"))
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
		panic(fmt.Sprintf("write content into file:%s form template error:%s", path+name, e))
	}
}
