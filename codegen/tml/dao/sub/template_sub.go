package sub

import (
	"fmt"
	"os"
	"text/template"
)

const text = `package {{.}}

import (
	"database/sql"

	"github.com/chenjie199234/Corelib/redis"
)

//Dao this is a data operation layer to operate {{.}} service's data
type Dao struct {
	db    *sql.DB
	cache *redis.Pool
}

//NewDao Dao is only a data operation layer
//don't write business logic in this package
//business logic should be written in service package
func NewDao(db *sql.DB, cache *redis.Pool) *Dao {
	return &Dao{
		db:    db,
		cache: cache,
	}
}`
const textsql = `package {{.}}`
const textredis = `package {{.}}`

const path = "./dao/"
const name = "dao.go"
const namesql = "sql.go"
const nameredis = "redis.go"

var tml *template.Template
var tmlsql *template.Template
var tmlredis *template.Template
var file *os.File
var filesql *os.File
var fileredis *os.File

type data struct {
	Pname string
	Sname string
}

func init() {
	var e error
	tml, e = template.New("dao").Parse(text)
	if e != nil {
		panic(fmt.Sprintf("create template for subservice error:%s", e))
	}
	tmlsql, e = template.New("sql").Parse(textsql)
	if e != nil {
		panic(fmt.Sprintf("create template for subservice error:%s", e))
	}
	tmlredis, e = template.New("redis").Parse(textredis)
	if e != nil {
		panic(fmt.Sprintf("create template for subservice error:%s", e))
	}
}
func CreatePathAndFile(sname string) {
	var e error
	if e = os.MkdirAll(path+sname+"/", 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+sname+"/"+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+"/"+name, e))
	}
	filesql, e = os.OpenFile(path+sname+"/"+namesql, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+"/"+namesql, e))
	}
	fileredis, e = os.OpenFile(path+sname+"/"+nameredis, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+sname+"/"+nameredis, e))
	}
}
func Execute(sname string) {
	if e := tml.Execute(file, sname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+sname+"/"+name, e))
	}
	if e := tmlsql.Execute(filesql, sname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+sname+"/"+namesql, e))
	}
	if e := tmlredis.Execute(fileredis, sname); e != nil {
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+sname+"/"+nameredis, e))
	}
}