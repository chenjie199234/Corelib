package redis

import (
	"context"
	"testing"
)

func Test_Redis(t *testing.T) {
	c := &Config{}
	db, e := NewRedis(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	if _, e = db.Ping(context.Background()).Result(); e != nil {
		t.Fatal(e)
		return
	}
	return
}
