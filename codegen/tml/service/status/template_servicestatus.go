package status

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package status

import (
	"context"
	"time"

	//"{{.}}/source"
	"{{.}}/api"
	statusdao "{{.}}/dao/status"
)

//Service subservice for status business
type Service struct {
	statusDao *statusdao.Dao
}

//Start -
func Start() *Service {
	return &Service{
		//statusDao: statusdao.NewDao(source.GetDB("status_db"), source.GetRedis("status_redis")),
		statusDao: statusdao.NewDao(nil, nil),
	}
}

func (s *Service) Ping(context.Context, *api.Pingreq) (*api.Pingresp, error) {
	return &api.Pingresp{Timestamp: time.Now().UnixNano()}, nil
}

//Stop -
func (s *Service) Stop() {

}`

const path = "./service/status/"
const name = "status.go"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("status").Parse(text)
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
