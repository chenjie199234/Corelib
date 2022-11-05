package redis

import (
	"context"
	"testing"
	"time"
)

func Test_Ratelimit(t *testing.T) {
	client := NewRedis(&Config{
		URL:         "redis://127.0.0.1:6379",
		MaxOpen:     100,
		MaxIdletime: time.Second,
		ConnTimeout: time.Second,
		IOTimeout:   time.Second,
	})
	pass, e := client.RateLimitSecondMax(context.Background(), map[string]uint64{"test1": 2, "test2": 1})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	pass, e = client.RateLimitSecondMax(context.Background(), map[string]uint64{"test1": 2, "test2": 1})
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	pass, e = client.RateLimitSecondMax(context.Background(), map[string]uint64{"test1": 2, "test2": 2})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	time.Sleep(time.Millisecond * 400)
	pass, e = client.RateLimitSecondMax(context.Background(), map[string]uint64{"test1": 3, "test2": 3})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	time.Sleep(time.Millisecond * 601)
	pass, e = client.RateLimitSecondMax(context.Background(), map[string]uint64{"test1": 2, "test2": 2})
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	conn, e := client.GetContext(context.Background())
	if e != nil {
		t.Fatal(e)
		return
	}
	count, e := Int(conn.DoContext(context.Background(), "LLEN", "test1"))
	if e != nil {
		t.Fatal(e)
		return
	}
	if count != 2 {
		t.Fatal("should left 2 in the rate limit list")
		return
	}
}
