package mysql

import (
	"context"
	"testing"
)

func Test_Mysql(t *testing.T) {
	c := &Config{}
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
