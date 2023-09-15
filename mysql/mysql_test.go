package mysql

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Mysql(t *testing.T) {
	c := &Config{
		MysqlName:       "test",
		Addr:            "127.0.0.1:3306",
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
