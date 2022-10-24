package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Token(t *testing.T) {
	UpdateTokenConfig("123", time.Second)
	tokenstr := MakeToken(context.Background(), "corelib", "ali", "test", "data")
	token := VerifyToken(context.Background(), tokenstr)
	if token == nil {
		t.Fatal("should verify token success")
	}
	UpdateTokenConfig("abc", time.Second)
	token = VerifyToken(context.Background(), tokenstr)
	if token != nil {
		t.Fatal("should not verify token success")
	}
	UpdateTokenConfig("123", time.Second)
	time.Sleep(time.Second * 2)
	token = VerifyToken(context.Background(), tokenstr)
	if token != nil {
		t.Fatal("should not verify token success")
	}
}
