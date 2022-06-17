package mids

import (
	"testing"
	"time"
)

func Test_Token(t *testing.T) {
	now := time.Now()
	tokenstr := MakeToken("sec", "corelib", "ali", "test", "data", uint64(now.Unix()), uint64(now.Add(time.Second).Unix()))
	_, e := VerifyToken("sec1", tokenstr)
	if e == nil {
		panic("should not verify token success")
	}
	_, e = VerifyToken("sec", tokenstr)
	if e != nil {
		panic("should verify token success")
	}
	time.Sleep(time.Second * 2)
	_, e = VerifyToken("sec", tokenstr)
	if e == nil {
		panic("should not verify token success")
	}
}
