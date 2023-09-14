package redis

import (
	"context"
	"testing"
	"time"
)

func Test_Redis(t *testing.T) {
	c := &Config{
		RedisName:       "test",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: time.Second,
		DialTimeout:     time.Second,
		IOTimeout:       time.Second,
	}
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
