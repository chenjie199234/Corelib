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
	pass, e := client.RateLimitSecondMax(context.Background(), "test", 2)
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	pass, e = client.RateLimitSecondMax(context.Background(), "test", 2)
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	pass, e = client.RateLimitSecondMax(context.Background(), "test", 2)
	if e != nil {
		t.Fatal(e)
		return
	}
	if pass {
		t.Fatal("should not pass")
		return
	}
	time.Sleep(time.Millisecond * 100)
	pass, e = client.RateLimitSecondMax(context.Background(), "test", 3)
	if e != nil {
		t.Fatal(e)
		return
	}
	if !pass {
		t.Fatal("should pass")
		return
	}
	time.Sleep(time.Millisecond * 901)
	pass, e = client.RateLimitSecondMax(context.Background(), "test", 2)
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
	count, e := Int(conn.DoContext(context.Background(), "LLEN", "test"))
	if e != nil {
		t.Fatal(e)
		return
	}
	if count != 2 {
		t.Fatal("should left 2 in the rate limit list")
		return
	}
}
