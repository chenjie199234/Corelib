package sub

import (
	"os"
	"text/template"
)

const dao = `package {{.}}

import (
	credis "github.com/chenjie199234/Corelib/redis"
	cmongo "github.com/chenjie199234/Corelib/mongo"
	cmysql "github.com/chenjie199234/Corelib/mysql"
)

// Dao this is a data operation layer to operate {{.}} service's data
type Dao struct {
	mysql *cmysql.Client
	redis *credis.Client
	mongo *cmongo.Client
}

// NewDao Dao is only a data operation layer
// don't write business logic in this package
// business logic should be written in service package
func NewDao(mysql *cmysql.Client, redis *credis.Client, mongo *cmongo.Client) *Dao {
	return &Dao{
		mysql: mysql,
		redis: redis,
		mongo: mongo,
	}
}`
const sql = `package {{.}}`
const redis = `package {{.}}`
const mongo = `package {{.}}`

func CreatePathAndFile(sname string) {
	if e := os.MkdirAll("./dao/"+sname+"/", 0755); e != nil {
		panic("mkdir ./dao/" + sname + "/ error: " + e.Error())
	}
	//dao.go
	daotemplate, e := template.New("./dao/" + sname + "/dao.go").Parse(dao)
	if e != nil {
		panic("parse ./dao/" + sname + "/dao.go template error: " + e.Error())
	}
	daofile, e := os.OpenFile("./dao/"+sname+"/dao.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./dao/" + sname + "/dao.go error: " + e.Error())
	}
	if e := daotemplate.Execute(daofile, sname); e != nil {
		panic("write ./dao/" + sname + "/dao.go error: " + e.Error())
	}
	if e := daofile.Sync(); e != nil {
		panic("sync ./dao/" + sname + "/dao.go error: " + e.Error())
	}
	if e := daofile.Close(); e != nil {
		panic("close ./dao/" + sname + "/dao.go error: " + e.Error())
	}
	//sql.go
	sqltemplate, e := template.New("./dao/" + sname + "/sql.go").Parse(sql)
	if e != nil {
		panic("parse ./dao/" + sname + "/sql.go template error: " + e.Error())
	}
	sqlfile, e := os.OpenFile("./dao/"+sname+"/sql.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./dao/" + sname + "/sql.go error: " + e.Error())
	}
	if e := sqltemplate.Execute(sqlfile, sname); e != nil {
		panic("write ./dao/" + sname + "/sql.go error: " + e.Error())
	}
	if e := sqlfile.Sync(); e != nil {
		panic("sync ./dao/" + sname + "/sql.go error: " + e.Error())
	}
	if e := sqlfile.Close(); e != nil {
		panic("close ./dao/" + sname + "/sql.go error: " + e.Error())
	}
	//mongo.go
	mongotemplate, e := template.New("./dao/" + sname + "/mongo.go").Parse(mongo)
	if e != nil {
		panic("parse ./dao/" + sname + "/mongo.go template error: " + e.Error())
	}
	mongofile, e := os.OpenFile("./dao/"+sname+"/mongo.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./dao/" + sname + "/mongo.go error: " + e.Error())
	}
	if e := mongotemplate.Execute(mongofile, sname); e != nil {
		panic("write ./dao/" + sname + "/mongo.go error: " + e.Error())
	}
	if e := mongofile.Sync(); e != nil {
		panic("sync ./dao/" + sname + "/mongo.go error: " + e.Error())
	}
	if e := mongofile.Close(); e != nil {
		panic("close ./dao/" + sname + "/mongo.go error: " + e.Error())
	}
	//redis.go
	redistemplate, e := template.New("./dao/" + sname + "/redis.go").Parse(redis)
	if e != nil {
		panic("parse ./dao/" + sname + "/redis.go template error: " + e.Error())
	}
	redisfile, e := os.OpenFile("./dao/"+sname+"/redis.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./dao/" + sname + "/redis.go error: " + e.Error())
	}
	if e := redistemplate.Execute(redisfile, sname); e != nil {
		panic("write ./dao/" + sname + "/redis.go error: " + e.Error())
	}
	if e := redisfile.Sync(); e != nil {
		panic("sync ./dao/" + sname + "/redis.go error: " + e.Error())
	}
	if e := redisfile.Close(); e != nil {
		panic("close ./dao/" + sname + "/redis.go error: " + e.Error())
	}
}
