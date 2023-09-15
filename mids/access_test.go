package mids

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/redis"
	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Accesssign(t *testing.T) {
	client, _ := redis.NewRedis(&redis.Config{
		RedisName:       "test",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	UpdateReplayDefendRedisInstance(client)
	UpdateAccessConfig(MultiPathAccessConfigs{
		"/abc": {
			{Methods: []string{"GET", "GRPC", "CRPC"}, Accesses: map[string]string{"1": "1"}},
		},
	})
	querys := make(url.Values)
	querys.Add("a", "我 = 们")
	querys.Add("a", "ad = asd")
	querys.Add("b", "b")
	headers := make(http.Header)
	headers.Add("header1", "  header1  ")
	headers.Add("header1", "我们  ")
	headers.Add("header2", "  你们")
	metadata := make(map[string]string)
	metadata["md1"] = "  md1  "
	metadata["md2"] = "  我们 "
	signstr := MakeAccessSign("1", "1", "GRPC", "/abc", querys, headers, metadata, nil)
	t.Log(signstr)
	headers = make(http.Header)
	headers.Add("header1", "header1")
	headers.Add("header1", "我们")
	headers.Add("header2", "你们")
	if !VerifyAccessSign(context.Background(), "GRPC", "/abc", querys, headers, metadata, nil, signstr) {
		t.Fatal("should pass")
	}
	if VerifyAccessSign(context.Background(), "GRPC", "/abc", querys, headers, metadata, nil, signstr) {
		t.Fatal("should not pass")
	}
	if VerifyAccessSign(context.Background(), "GET", "/abc", querys, headers, metadata, nil, signstr) {
		t.Fatal("should not pass")
	}
	if VerifyAccessSign(context.Background(), "GRPC", "/a", querys, headers, metadata, nil, signstr) {
		t.Fatal("should not pass")
	}
}
