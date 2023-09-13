package redis

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test_TemporaryMQ(t *testing.T) {
	client, _ := NewRedis(&Config{
		RedisName:       "test",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: time.Minute * 5,
		DialTimeout:     time.Second,
		IOTimeout:       time.Second,
	}, nil)
	lker := &sync.Mutex{}
	r := make(map[string]int, 1000)
	done := make(chan *struct{}, 1)
	cancel, e := client.TemporaryMQSub("test", 20, func(data []byte) {
		lker.Lock()
		if v, ok := r[string(data)]; ok {
			r[string(data)] = v + 1
		} else {
			r[string(data)] = 1
		}
		if len(r) == 1000 {
			select {
			case done <- nil:
			default:
			}
		}
		lker.Unlock()
	})
	if e != nil {
		t.Fatal(e)
	}
	//wait 30s the test the suber's refresh
	time.Sleep(time.Second * 30)
	for i := 0; i < 1000; i++ {
		e := client.TemporaryMQPub(context.Background(), "test", 20, strconv.Itoa(i), strconv.AppendInt(nil, int64(i), 10))
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Log("start sub")
	<-done
	t.Log("finish sub")
	cancel()
	for i := 0; i < 1000; i++ {
		s := strconv.Itoa(i)
		v, ok := r[s]
		if !ok {
			t.Fatal("missing: " + s)
		} else if v != 1 {
			t.Fatal("count wrong: " + s + " count: " + strconv.Itoa(v))
		}
	}
}
