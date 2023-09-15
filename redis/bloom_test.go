package redis

import (
	"context"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Bloom(t *testing.T) {
	client, _ := NewRedis(&Config{
		RedisName:       "test",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	e := client.NewBloom(context.Background(), "testbloom", 10, 1024, 86400)
	if e != nil {
		t.Fatal("new bloom failed:" + e.Error())
		return
	}
	status, e := client.SetBloom(context.Background(), "testbloom", 10, 1024, "testkey1")
	if e != nil {
		t.Fatal("set bloom failed:" + e.Error())
		return
	}
	if !status {
		t.Fatal("should set success")
		return
	}
	status, e = client.CheckBloom(context.Background(), "testbloom", 10, 1024, "testkey1")
	if e != nil {
		t.Fatal("check bloom failed:" + e.Error())
		return
	}
	if status {
		t.Fatal("should check failed")
		return
	}
	status, e = client.CheckBloom(context.Background(), "testbloom", 10, 1024, "testkey2")
	if e != nil {
		t.Fatal("check bloom failed:" + e.Error())
		return
	}
	if !status {
		t.Fatal("should check success")
		return
	}
	if e := client.DelBloom(context.Background(), "testbloom", 10); e != nil {
		t.Fatal("del bloom failed:" + e.Error())
		return
	}
}
