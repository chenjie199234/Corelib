package discovery

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

const text = `package discovery

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"
	
	//make sure source package is the first init
	_ "{{.}}/source"

	"github.com/fsnotify/fsnotify"
	"gitlab.lilithgame.com/golang/lilin/pkg/log"
)

//discoveryConfig can hot update
//this is the config used for update cluser info
type discoveryConfig struct {
	Http map[string][]string $json:"http"$ //key is the service name,value is a addr list
	Rpc  map[string][]string $json:"rpc"$  //key is the service name,value is a addr list
}

//DC -
var dc *discoveryConfig

var watcher *fsnotify.Watcher
var closech chan struct{}
var noticehttp map[chan struct{}]struct{}
var noticerpc map[chan struct{}]struct{}
var lker sync.Mutex

func init() {
	data, e := ioutil.ReadFile("DiscoveryConfig.json")
	if e != nil {
		log.Fatalf("[DiscoveryConfig]read config file error:%s", e)
	}
	dc = &discoveryConfig{}
	if e = json.Unmarshal(data, dc); e != nil {
		log.Fatalf("[DiscoveryConfig]config data format error:%s", e)
	}
	watcher, e = fsnotify.NewWatcher()
	if e != nil {
		log.Fatalf("[DiscoveryConfig]create watcher for hot update error:%s", e)
	}
	if e = watcher.Add("./"); e != nil {
		log.Fatalf("[DiscoveryConfig]create watcher for hot update error:%s", e)
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
			if filepath.Base(event.Name) != "DiscoveryConfig.json" || (event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Write == 0) {
				continue
			}
			data, e := ioutil.ReadFile("DiscoveryConfig.json")
			if e != nil {
				log.Errorf("[DiscoveryConfig]hot update read config file error:%s", e)
				continue
			}
			c := &discoveryConfig{}
			if e = json.Unmarshal(data, c); e != nil {
				log.Errorf("[DiscoveryConfig]hot update config data format error:%s", e)
				continue
			}
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&dc)), unsafe.Pointer(c))
			lker.Lock()
			for ch := range noticehttp {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			for ch := range noticerpc {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			lker.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Errorf("[DiscoveryConfig]watcher error:%s", err)
		}
	}
}
func Close() {
	watcher.Close()
	<-closech
}
func GetHttpAddrs(servicename string) []string {
	temp := (*discoveryConfig)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&dc))))
	return temp.Http[servicename]
}
func NoiceHttp() <-chan struct{} {
	noticech := make(chan struct{}, 1)
	lker.Lock()
	defer lker.Unlock()
	noticehttp[noticech] = struct{}{}
	return noticech
}
func GetRpcAddrs(servicename string) []string {
	temp := (*discoveryConfig)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&dc))))
	return temp.Rpc[servicename]
}
func NoticeGrpc() <-chan struct{} {
	noticech := make(chan struct{}, 1)
	lker.Lock()
	defer lker.Unlock()
	noticerpc[noticech] = struct{}{}
	return noticech
}`
const path = "./discovery/"
const name = "discovery.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("discovery").Parse(strings.ReplaceAll(text, "$", "`"))
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
