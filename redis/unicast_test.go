package redis

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"

	gredis "github.com/redis/go-redis/v9"
)

func Test_Unicast(t *testing.T) {
	client, _ := NewRedis(&Config{
		RedisName:       "test",
		RedisMode:       "direct",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	lker := &sync.Mutex{}
	r := make(map[string]int, 1000)
	done := make(chan *struct{}, 1)
	cancel, e := client.SubUnicast("test", 20, func(data []byte) {
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
	time.Sleep(time.Second * 3)
	for i := 0; i < 1000; i++ {
		e := client.PubUnicast(context.Background(), "test", 20, strconv.Itoa(i), strconv.AppendInt(nil, int64(i), 10))
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
	time.Sleep(time.Second * 3)
	if e := client.PubUnicast(context.Background(), "test", 20, "", "a"); e != gredis.Nil {
		t.Fatal("sub already stopped,pub should return nil", e)
	}
}
