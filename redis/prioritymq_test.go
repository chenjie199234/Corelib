package redis

import (
	"context"
	"strconv"
	"time"

	"testing"
)

func Test_PriorityMQ(t *testing.T) {
	pool := NewRedis(&Config{
		RedisName:   "test",
		URL:         "redis://127.0.0.1:6379",
		MaxOpen:     100,
		MaxIdletime: time.Minute * 10,
		ConnTimeout: time.Second,
		IOTimeout:   time.Second,
	})
	pubfinished := make(map[string]bool)
	pubfinished["testtask"] = false
	go func() {
		//global checker
		//this should be used by puber
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			tasks, e := pool.PriorityMQGetCurTasks(context.Background(), "testgroup")
			if e != nil {
				panic(e)
			}
			for taskname := range tasks {
				if !pubfinished[taskname] {
					continue
				}
				r, e := pool.PriorityMQFinishTask(context.Background(), "testgroup", "testtask", "testtopic_1", "testtopic_2")
				if e != nil {
					panic(e)
				}
				if r == 0 {
					t.Log("pub finished but sub not finished:", taskname)
				} else {
					t.Log("pub and sub finished:", taskname)
				}
			}
		}
	}()
	if e := pool.PriorityMQSetTask(context.Background(), "testgroup", "testtask", 1); e != nil {
		t.Fatal(e)
		return
	}
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			pool.PriorityMQPub(context.Background(), "testgroup", "testtask", "testtopic_2", strconv.AppendInt(nil, int64(i), 10))
		} else {
			pool.PriorityMQPub(context.Background(), "testgroup", "testtask", "testtopic_1", strconv.AppendInt(nil, int64(i), 10))
		}
	}
	pubfinished["testtask"] = true
	//this function should be called by puber
	if r, e := pool.PriorityMQFinishTask(context.Background(), "testgroup", "testtask", "testtopic_1", "testtopic_2"); e != nil {
		t.Fatal(e)
		return
	} else if r == 0 {
		t.Log("pub finished but sub not finished:", "testtask")
	} else {
		t.Log("pub and sub finished:", "testtask")
	}
	cancel1, e := pool.PriorityMQSub("testgroup", "testtopic_1", func(taskname, taskdata string) {
		t.Log(taskname, ":", taskdata, ":", "testtopic_1")
	})
	if e != nil {
		t.Fatal(e)
		return
	}
	cancel2, e := pool.PriorityMQSub("testgroup", "testtopic_2", func(taskname, taskdata string) {
		t.Log(taskname, ":", taskdata, ":", "testtopic_2")
	})
	if e != nil {
		t.Fatal(e)
		return
	}
	time.Sleep(time.Second * 5)
	cancel1()
	cancel2()
}
