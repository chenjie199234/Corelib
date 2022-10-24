package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Rate(t *testing.T) {
	UpdateRateConfig("redis://127.0.0.1:6379", []*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass := HttpGetRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	if pass := GrpcRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check")
	}
	time.Sleep(time.Millisecond * 100)
	//rest the single,but global will not rest
	UpdateRateConfig("redis://127.0.0.1:6379", []*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass := HttpPostRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	//rest the single,but global will not rest
	UpdateRateConfig("redis://127.0.0.1:6379", []*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, SingleMaxPerSec: 1, GlobalMaxPerSec: 2}})
	if pass := HttpPutRate(context.Background(), "/abc"); pass {
		t.Fatal("should not pass rate check")
	}
	time.Sleep(time.Millisecond * 901)
	if pass := CrpcRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
}
