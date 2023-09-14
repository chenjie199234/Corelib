package mysql

import (
	"context"
	"testing"
	"time"
)

func Test_Mysql(t *testing.T) {
	c := &Config{
		MysqlName:       "test",
		Addr:            "127.0.0.1:3306",
		ParseTime:       true,
		MaxOpen:         256,
		MaxConnIdletime: time.Minute * 5,
		DialTimeout:     time.Second,
		IOTimeout:       time.Second,
	}
	db, e := NewMysql(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	if e = db.PingContext(context.Background()); e != nil {
		t.Fatal(e)
		return
	}
	return
}
