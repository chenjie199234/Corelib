package mysql

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Mysql(t *testing.T) {
	c := &Config{
		MysqlName: "test",
		Master: &struct {
			Addr     string `json:"addr"`
			UserName string `json:"user_name"`
			Password string `json:"password"`
		}{
			Addr:     "127.0.0.1:3306",
			UserName: "root",
			Password: "",
		},
		Slaves: &struct {
			Addrs    []string `json:"addrs"`
			UserName string   `json:"user_name"`
			Password string   `json:"password"`
		}{
			Addrs:    []string{},
			UserName: "",
			Password: "",
		},
		ParseTime:       true,
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}
	_, e := NewMysql(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
