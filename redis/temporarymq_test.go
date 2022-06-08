package redis

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func Test_ListMQ(t *testing.T) {
	pool := NewRedis(&Config{
		RedisName:   "test",
		URL:         "redis://127.0.0.1:6379",
		MaxOpen:     100,
		MaxIdletime: time.Minute * 10,
		ConnTimeout: time.Second,
		IOTimeout:   time.Second,
	})
	cancel, e := pool.TemporaryMQSub("test", 31, func(data []byte) {
		t.Log(string(data))
	})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		e := pool.TemporaryMQPub(context.Background(), "test", strconv.Itoa(i), strconv.AppendInt(nil, int64(i), 10))
		if e != nil {
			t.Fatal(e)
		}
	}
	time.Sleep(time.Second * 5)
	cancel()
}
