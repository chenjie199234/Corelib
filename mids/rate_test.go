package mids

import (
	"context"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/redis"
)

func Test_SelfRate(t *testing.T) {
	UpdateSelfRateConfig([]*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxPerSec: 1}})
	if pass, _ := HttpGetRate(context.Background(), "/abc", false); !pass {
		panic("should pass rate check")
	}
	if pass, _ := GrpcRate(context.Background(), "/abc", false); pass {
		panic("should not pass rate check")
	}
	if pass, _ := HttpGetRate(context.Background(), "/abc", false); pass {
		panic("should not pass rate check")
	}
	time.Sleep(time.Second * 2)
	if pass, _ := GrpcRate(context.Background(), "/abc", false); !pass {
		panic("should pass rate check")
	}
}
func Test_GlobalRate(t *testing.T) {
	UpdateGlobalRateConfig(&redis.Config{
		URL:         "redis://127.0.0.1:6379",
		MaxOpen:     100,
		MaxIdletime: time.Second,
		IOTimeout:   time.Second,
		ConnTimeout: time.Second,
	}, []*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxPerSec: 2}})
	if pass, e := GrpcRate(context.Background(), "/abc", true); e != nil {
		panic(e)
	} else if !pass {
		panic("should pass rate check")
	}
	time.Sleep(time.Millisecond * 100)
	if pass, e := HttpGetRate(context.Background(), "/abc", true); e != nil {
		panic(e)
	} else if !pass {
		panic("should pass rate check")
	}
	if pass, e := HttpPostRate(context.Background(), "/abc", true); e != nil {
		panic(e)
	} else if pass {
		panic("should not pass rate check")
	}
	time.Sleep(time.Millisecond * 901)
	if pass, e := HttpPatchRate(context.Background(), "/abc", true); e != nil {
		panic(e)
	} else if !pass {
		panic("should pass rate check")
	}
	if pass, e := HttpPutRate(context.Background(), "/abc", true); e != nil {
		panic(e)
	} else if pass {
		panic("should not pass rate check")
	}
}
