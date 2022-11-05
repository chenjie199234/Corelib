package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Session(t *testing.T) {
	UpdateSessionRedisUrl("redis://127.0.0.1:6379")
	UpdateSessionConfig(time.Second)
	sessionstr := MakeSession(context.Background(), "1", "123")
	if sessionstr == "" {
		t.Fatal("should make session success")
	}
	data, status := VerifySession(context.Background(), sessionstr)
	if !status {
		t.Fatal("should verify session success")
	}
	if data != "123" {
		t.Fatal("session data broken")
	}
	time.Sleep(time.Second)
	data, status = VerifySession(context.Background(), sessionstr)
	if status {
		t.Fatal("should not verify success")
	}
	sessionstr = MakeSession(context.Background(), "1", "123")
	if sessionstr == "" {
		t.Fatal("should make session success")
	}
	time.Sleep(time.Millisecond * 900)
	if !ExtendSession(context.Background(), "1", time.Second) {
		t.Fatal("should extend session success")
	}
	time.Sleep(time.Millisecond * 500)
	data, status = VerifySession(context.Background(), sessionstr)
	if !status {
		t.Fatal("should verify success")
	}
	if data != "123" {
		t.Fatal("session data broken")
	}
	if !CleanSession(context.Background(), "1") {
		t.Fatal("should clean success")
	}
	data, status = VerifySession(context.Background(), sessionstr)
	if status {
		t.Fatal("should not verify success")
	}
}
