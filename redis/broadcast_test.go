package redis

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Broadcast(t *testing.T) {
	client, e := NewRedis(&Config{
		RedisName:       "test",
		RedisMode:       "direct",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	if e != nil {
		t.Fatal("new redis failed:", e)
		return
	}
	ch := make(chan *struct{}, 3)
	suber1 := make(map[string]int, 10000)
	lker1 := sync.Mutex{}
	suber2 := make(map[string]int, 10000)
	lker2 := sync.Mutex{}
	suber3 := make(map[string]int, 10000)
	lker3 := sync.Mutex{}
	for i := range 10000 {
		suber1[strconv.Itoa(i)] = i
		suber2[strconv.Itoa(i)] = i
		suber3[strconv.Itoa(i)] = i
	}
	if _ = client.SubBroadcast("testbroadcast", 2, func(values map[string]any, last bool) {
		//suber 1
		lker1.Lock()
		defer func() {
			if len(suber1) == 0 {
				ch <- nil
			}
			lker1.Unlock()
		}()
		if len(values) != 1 {
			e = errors.New("data broken")
			return
		}
		for k, v := range values {
			delete(suber1, k)
			if v.(string) != k {
				e = errors.New("data broken")
				return
			}
		}
	}); e != nil {
		t.Fatal("sub 1 failed:", e)
		return
	}
	if _ = client.SubBroadcast("testbroadcast", 2, func(values map[string]any, last bool) {
		//suber 2
		lker2.Lock()
		defer func() {
			if len(suber2) == 0 {
				ch <- nil
			}
			lker2.Unlock()
		}()
		if len(values) != 1 {
			e = errors.New("data broken")
			return
		}
		for k, v := range values {
			delete(suber2, k)
			if v.(string) != k {
				e = errors.New("data broken")
				return
			}
		}
	}); e != nil {
		t.Fatal("sub 2 failed:", e)
		return
	}
	if _ = client.SubBroadcast("testbroadcast", 2, func(values map[string]any, last bool) {
		//suber 3
		lker3.Lock()
		defer func() {
			if len(suber3) == 0 {
				ch <- nil
			}
			lker3.Unlock()
		}()
		if len(values) != 1 {
			e = errors.New("data broken")
			return
		}
		for k, v := range values {
			delete(suber3, k)
			if v.(string) != k {
				e = errors.New("data broken")
				return
			}
		}
	}); e != nil {
		t.Fatal("sub 3 failed:", e)
		return
	}
	time.Sleep(time.Second)
	var timestamp int
	for i := range 10000 {
		if i == 5000 {
			timestamp = int(time.Now().Unix()) + 1
			time.Sleep(time.Second * 2)
		}
		str := strconv.Itoa(i)
		if e := client.PubBroadcast(context.Background(), "testbroadcast", 2, str, map[string]any{str: i}); e != nil {
			t.Fatal("pub failed:", e)
		}
	}
	<-ch
	<-ch
	<-ch
	if e != nil {
		t.Fatal(e)
		return
	}
	stream0, e := client.XLen(context.Background(), "broadcast_testbroadcast_0").Result()
	if e != nil {
		t.Fatal("get stream len failed:", e)
		return
	}
	stream1, e := client.XLen(context.Background(), "broadcast_testbroadcast_1").Result()
	if e != nil {
		t.Fatal("get stream len failed:", e)
		return
	}
	if e := client.TrimBroadcast(context.Background(), "testbroadcast", 2, uint64(timestamp)); e != nil {
		t.Fatal("trim failed:", e)
		return
	}
	stream00, e := client.XLen(context.Background(), "broadcast_testbroadcast_0").Result()
	if e != nil {
		t.Fatal("get stream len failed:", e)
		return
	}
	stream11, e := client.XLen(context.Background(), "broadcast_testbroadcast_1").Result()
	if e != nil {
		t.Fatal("get stream len failed:", e)
		return
	}
	if stream00 == stream0 || stream11 == stream1 {
		t.Fatal("trim failed: old 0:", stream0, " new 0:", stream00, " old 1:", stream1, " old 11:", stream11)
		return
	}
	if e := client.DelBroadcast(context.Background(), "testbroadcast", 2); e != nil {
		t.Fatal("del failed:", e)
		return
	}
}
