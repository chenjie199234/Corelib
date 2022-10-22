package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Rate(t *testing.T) {
	t.Setenv("RATE_REDIS_URL", "redis://127.0.0.1:6379")
	UpdateRateConfig([]*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass, e := HttpGetRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check:", e)
	}
	if pass, e := GrpcRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check:", e)
	}
	time.Sleep(time.Millisecond * 100)
	//rest the single,but global will not rest
	UpdateRateConfig([]*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass, e := HttpPostRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check:", e)
	}
	//rest the single,but global will not rest
	UpdateRateConfig([]*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass, e := HttpPutRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check:", e)
	}
	time.Sleep(time.Millisecond * 901)
	if pass, e := CrpcRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check:", e)
	}
}
