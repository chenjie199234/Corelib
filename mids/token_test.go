package mids

import (
	"context"
	"testing"
	"time"
)

func Test_Token(t *testing.T) {
	userid := "userid1"
	data := "dajsldjlashdohasobncasbjkdhaosjdoasjdoiajsodjnlajsnckjbakjsdashdjkasdoqhwofhbihdbvisadofjaoasdjlasjdlajsldjoaisjdoajsodijasdjlaksjdlkajsldjlaksjdlaslsdlaksjdlajsldjalsjdlasjdlajlsdjalsjdlasjdlasjdlkasjldkajsldkjalskdjlkjdlsjdoas"
	UpdateTokenConfig("123")
	tokenstr := MakeToken(context.Background(), "corelib", "ali", "test", userid, data, time.Second)
	if tokenstr == "" {
		t.Fatal("should make token success")
	}
	token := VerifyToken(context.Background(), tokenstr)
	if token == nil {
		t.Fatal("should verify token success")
	}
	if token.UserID != userid {
		t.Fatal("data broken")
	}
	if token.Data != data {
		t.Fatal("data broken")
	}
	UpdateTokenConfig("abc")
	token = VerifyToken(context.Background(), tokenstr)
	if token != nil {
		t.Fatal("should not verify token success")
	}
	UpdateTokenConfig("123")
	time.Sleep(time.Second * 2)
	token = VerifyToken(context.Background(), tokenstr)
	if token != nil {
		t.Fatal("should not verify token success")
	}
}
