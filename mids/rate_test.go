package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Rate(t *testing.T) {
	UpdateRateRedisUrl("redis://127.0.0.1:6379")
	UpdateRateConfig(map[string][]*PathRateConfig{
		"/abc": {
			{Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxPerSec: 2},
			{Method: []string{"GRPC"}, MaxPerSec: 1},
		},
	})
	if pass := HttpGetRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	if pass := GrpcRate(context.Background(), "/abc"); !pass {
		t.Fatal("should pass rate check")
	}
	UpdateRateConfig(map[string][]*PathRateConfig{
		"/abc": {
			{Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxPerSec: 2},
			{Method: []string{"GRPC"}, MaxPerSec: 2},
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
