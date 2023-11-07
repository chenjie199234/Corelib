package redis

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_PriorityMQ(t *testing.T) {
	client, _ := NewRedis(&Config{
		RedisName:       "test",
		RedisMode:       "direct",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)

	// testtask's status is interrupted now
	if e := client.PriorityMQSetTask(context.Background(), "testgroup", "testtask", -1); e != nil {
		t.Fatal(e)
		return
	}
	go func() {
		//after some time,start the testtask
		time.Sleep(time.Second * 10)
		if e := client.PriorityMQSetTask(context.Background(), "testgroup", "testtask", 1); e != nil {
			t.Log("start testtask failed:" + e.Error())
			return
		}
		t.Log("can sub now")
	}()
	channel1 := make([]interface{}, 0, 500)
	channel2 := make([]interface{}, 0, 500)
	for i := 0; i < 1000; i++ {
		if i%2 == 0 {
			channel1 = append(channel1, strconv.AppendInt(nil, int64(i), 10))
		} else {
			channel2 = append(channel2, strconv.AppendInt(nil, int64(i), 10))
		}
	}
	if e := client.PriorityMQPub(context.Background(), "testgroup", "testtask", "testchannel_1", channel1...); e != nil {
		t.Fatal("pub data failed:" + e.Error())
		return
	}
	t.Log("finish pub channel1")
	if e := client.PriorityMQPub(context.Background(), "testgroup", "testtask", "testchannel_2", channel2...); e != nil {
		t.Fatal("pub data failed:" + e.Error())
		return
	}
	t.Log("finish pub channel2")

	if r, e := client.PriorityMQFinishTaskPub(context.Background(), "testgroup", "testtask", "testchannel_1", "testchannel_2"); e != nil {
		t.Fatal("finish task pub failed:" + e.Error())
		return
	} else if r == 1 {
		t.Fatal("finish task pub should return 0")
		return
	}
	sub := make(map[int]*struct{}, 1000)
	done1 := make(chan *struct{}, 1)
	done2 := make(chan *struct{}, 1)
	cancel1, e := client.PriorityMQSub("testgroup", "testchannel_1", func(task string, data []byte) {
		if task != "testtask" {
			t.Fatal("task name broken")
			return
		}
		d, e := strconv.Atoi(string(data))
		if e != nil {
			t.Fatal("channel1 data broken")
			return
		}
		if d%2 != 0 || d > 1000 || d < 0 {
			t.Fatal("channel1 data broken")
			return
		}
		sub[d] = nil
		if len(sub) == 500 {
			done1 <- nil
		}
	})
	if e != nil {
		t.Fatal("sub channel1 failed:" + e.Error())
		return
	}
	t.Log("start sub channel1")
	<-done1
	t.Log("finish sub channel1")
	if r, e := client.PriorityMQFinishTaskPub(context.Background(), "testgroup", "testtask", "testchannel_1", "testchannel_2"); e != nil {
		t.Fatal("finish task pub failed:" + e.Error())
		return
	} else if r == 1 {
		t.Fatal("finish task pub should return 0")
		return
	}
	cancel2, e := client.PriorityMQSub("testgroup", "testchannel_2", func(task string, data []byte) {
		if task != "testtask" {
			t.Fatal("task name broken")
			return
		}
		d, e := strconv.Atoi(string(data))
		if e != nil {
			t.Fatal("channel1 data broken")
			return
		}
		if d%2 != 1 || d > 1000 || d < 0 {
			t.Fatal("channel1 data broken")
			return
		}
		sub[d] = nil
		if len(sub) == 1000 {
			done2 <- nil
		}
	})
	if e != nil {
		t.Fatal("sub channel2 failed:" + e.Error())
		return
	}
	t.Log("start sub channel2")
	<-done2
	t.Log("finish sub channel2")
	if r, e := client.PriorityMQFinishTaskPub(context.Background(), "testgroup", "testtask", "testchannel_1", "testchannel_2"); e != nil {
		t.Fatal("finish task pub failed:" + e.Error())
		return
	} else if r == 0 {
		t.Fatal("finish task pub should return 1")
		return
	}
	cancel1()
	cancel2()
}
