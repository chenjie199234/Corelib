package mids

import (
	"context"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/redis"
)

func Test_Rate(t *testing.T) {
	client := redis.NewRedis(&redis.Config{
		RedisName:   "test",
		Addrs:       []string{"127.0.0.1:6379"},
		MaxIdle:     100,
		MaxOpen:     256,
		MaxIdletime: time.Minute * 5,
		ConnTimeout: time.Second,
		IOTimeout:   time.Second,
	}, nil)
	UpdateRateRedisInstance(client)
	UpdateRateConfig(MultiPathRateConfigs{
		"/abc": {
			{Methods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxRate: 2, Period: 1, RateType: "path"},
			{Methods: []string{"GRPC"}, MaxRate: 1, Period: 1, RateType: "path"},
		},
	})
	if pass := HttpGetRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	if pass := GrpcRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	UpdateRateConfig(MultiPathRateConfigs{
		"/abc": {
			{Methods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxRate: 2, Period: 1, RateType: "path"},
			{Methods: []string{"GRPC"}, MaxRate: 2, Period: 1, RateType: "path"},
		},
	})
	if pass := GrpcRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check")
	}
	if pass := HttpPostRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check")
	}
	time.Sleep(time.Millisecond * 1001)
	if pass := HttpPutRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	if pass := HttpPatchRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	if pass := GrpcRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check")
	}
}
