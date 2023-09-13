package redis

import (
	"context"
	"testing"
	"time"
)

func Test_Ratelimit(t *testing.T) {
	client := NewRedis(&Config{
		RedisName:   "test",
		Addrs:       []string{"127.0.0.1:6379"},
		MaxOpen:     256,
		MaxIdletime: time.Second,
		ConnTimeout: time.Second,
		IOTimeout:   time.Second,
	}, nil)
	pass, e := client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {1, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {1, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {2, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	time.Sleep(time.Second * 2)
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {3, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {3, 5}, "test2": {3, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	time.Sleep(time.Second * 4)
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {1, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {1, 5}, "test2": {2, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	pass, e = client.RateLimit(context.Background(), map[string][2]uint64{"test1": {2, 5}, "test2": {2, 5}})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	count, e := client.LLen(context.Background(), "test1").Result()
	if e != nil {
		t.Fatal(e)
		return
	}
	if count != 2 {
		t.Fatal("should left 2 in the rate limit list")
		return
	}
	count, e = client.LLen(context.Background(), "test2").Result()
	if e != nil {
		t.Fatal(e)
		return
	}
	if count != 2 {
		t.Fatal("should left 2 in the rate limit list")
	}
}
