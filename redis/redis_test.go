package redis

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Redis(t *testing.T) {
	c := &Config{
		RedisName:       "test",
		RedisMode:       "direct",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}
	_, e := NewRedis(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
