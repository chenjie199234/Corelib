package mids

import (
	"context"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/redis"
	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Session(t *testing.T) {
	client, _ := redis.NewRedis(&redis.Config{
		RedisName:       "test",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	UpdateSessionRedisInstance(client)
	sessionstr := MakeSession(context.Background(), "1", "123",time.Second)
	if sessionstr == "" {
		t.Fatal("should make session success")
	}
	userid, data, status := VerifySession(context.Background(), sessionstr)
	if !status {
		t.Fatal("should verify session success")
	}
	if userid != "1" {
		t.Fatal("session data broken")
	}
	if data != "123" {
		t.Fatal("session data broken")
	}
	time.Sleep(time.Second)
	userid, data, status = VerifySession(context.Background(), sessionstr)
	if status {
		t.Fatal("should not verify success")
	}
	sessionstr = MakeSession(context.Background(), "1", "123",time.Second)
	if sessionstr == "" {
		t.Fatal("should make session success")
	}
	time.Sleep(time.Millisecond * 900)
	if !ExtendSession(context.Background(), "1", time.Second) {
		t.Fatal("should extend session success")
	}
	time.Sleep(time.Millisecond * 500)
	userid, data, status = VerifySession(context.Background(), sessionstr)
	if !status {
		t.Fatal("should verify success")
	}
	if userid != "1" {
		t.Fatal("session data broken")
	}
	if data != "123" {
		t.Fatal("session data broken")
	}
	if !CleanSession(context.Background(), "1") {
		t.Fatal("should clean success")
	}
	userid, data, status = VerifySession(context.Background(), sessionstr)
	if status {
		t.Fatal("should not verify success")
	}
}
