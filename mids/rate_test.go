package mids

import (
	"testing"
	"time"
)

func Test_Rate(t *testing.T) {
	UpdateRateConfig([]*RateConfig{{Path: "/abc", Method: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "GRPC", "CRPC"}, MaxPerSec: 1}})
	if !HttpGetRate("/abc") {
		panic("should pass rate check")
	}
	if GrpcRate("/abc") {
		panic("should not pass rate check")
	}
	if HttpGetRate("/abc") {
		panic("should not pass rate check")
	}
	time.Sleep(time.Second * 2)
	if !GrpcRate("/abc") {
		panic("should pass rate check")
	}
}
