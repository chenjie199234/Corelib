package redis

import (
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
	_, e := NewRedis(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
