package bitfilter

import (
	//"context"
	"testing"
	//"time"
	//"github.com/gomodule/redigo/redis"
)

func Test_MemoryBitFilter(t *testing.T) {
	filter := NewMemoryBitFilter(1024)
	data := make([][]byte, 2)
	data[0] = []byte("12345")
	data[1] = []byte("abcde")
	filter.Set(data[0])
	filter.Set(data[1])
	if filter.Check(data[0]) {
		panic("data check error")
	}
	if filter.Check(data[1]) {
		panic("data check error")
	}
	datas, addnum := filter.Export()
	filter.Clear()
	newfilter := Rebuild(datas, addnum)
	if newfilter.Check(data[0]) {
		panic("data check error")
	}
	if newfilter.Check(data[1]) {
		panic("data check error")
	}
}

//func Test_RedisBitFilter(t *testing.T) {
//        pool := &redis.Pool{}
//        pool.Dial = func() (redis.Conn, error) {
//                return redis.Dial("tcp", "127.0.0.1:6379")
//        }
//        pool.MaxIdle = 10
//        pool.MaxActive = 10
//        pool.MaxConnLifetime = time.Hour

//        filter, e := NewRedisBitFilter(context.Background(), pool, "testbloom", &Option{})
//        if e != nil {
//                t.Fatal(e)
//        }
//        r, e := filter.Set(context.Background(), "test")
//        if e != nil {
//                t.Fatal(e)
//        }
//        if !r {
//                t.Fatal("add failed")
//        }
//        r, e = filter.Set(context.Background(), "test")
//        if e != nil {
//                t.Fatal(e)
//        }
//        if r {
//                t.Fatal("dup add failed")
//        }
//        rr, e := filter.Check(context.Background(), "test")
//        if e != nil {
//                t.Fatal(e)
//        }
//        if rr {
//                t.Fatal("check failed")
//        }
//        e = filter.clean()
//        if e != nil {
//                t.Fatal("clean error:" + e.Error())
//        }
//}
