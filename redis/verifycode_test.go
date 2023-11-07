package redis

import (
	"context"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_VerifyCode(t *testing.T) {
	client, e := NewRedis(&Config{
		RedisName:       "test",
		RedisMode:       "direct",
		Addrs:           []string{"127.0.0.1:6379"},
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	if e := client.DelVerifyCode(context.Background(), "testuserid", "testaction"); e != nil {
		t.Fatal(e)
		return
	}
	//1
	code, dup, e := client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if dup {
		t.Fatal("should not be dup")
		return
	}
	//1 dup check
	newcode, dup, e := client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "testemail@gmail.com", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if dup {
		t.Fatal("should not be dup")
		return
	}
	if code != newcode {
		t.Fatal("should get same code")
		return
	}
	newcode, dup, e = client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "testemail@gmail.com", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if !dup {
		t.Fatal("should be dup")
		return
	}
	if code != newcode {
		t.Fatal("should get same code")
		return
	}
	newcode, dup, e = client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "123123123", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if dup {
		t.Fatal("should not be dup")
		return
	}
	if code != newcode {
		t.Fatal("should get same code")
		return
	}

	//1 wrong receiver check
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", code, "abc"); e != ErrVerifyCodeReceiverMissing {
		if e == nil {
			t.Fatal("should not pass")
		} else {
			t.Fatal("should be receiver missing")
		}
		return
	}
	if e := client.HasCheckTimes(context.Background(), "testuserid", "testaction"); e != nil {
		t.Fatal(e)
		return
	}

	//1 no must receiver check
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", code, ""); e != nil {
		t.Fatal(e)
		return
	}
	if e := client.HasCheckTimes(context.Background(), "testuserid", "testaction"); e != ErrVerifyCodeMissing {
		t.Fatal("should be verify code missing")
		return
	}

	//2
	code, dup, e = client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "testemail@gmail.com", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if dup {
		t.Fatal("should not be dup")
		return
	}

	//2 must receiver check
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", code, "testemail@gmail.com"); e != nil {
		t.Fatal(e)
		return
	}
	if e := client.HasCheckTimes(context.Background(), "testuserid", "testaction"); e != ErrVerifyCodeMissing {
		t.Fatal("should be verify code missing")
		return
	}

	//3
	code, dup, e = client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "testemail@gmail.com", 300)
	if e != nil {
		t.Fatal(e)
		return
	}
	if dup {
		t.Fatal("should not be dup")
		return
	}

	//3 check times used up
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", "123", ""); e != ErrVerifyCodeWrong {
		if e == nil {
			t.Fatal("should not pass")
		} else {
			t.Fatal(e)
		}
		return
	}
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", "123", ""); e != ErrVerifyCodeWrong {
		if e == nil {
			t.Fatal("should not pass")
		} else {
			t.Fatal(e)
		}
		return
	}
	if e := client.HasCheckTimes(context.Background(), "testuserid", "testaction"); e != nil {
		t.Fatal(e)
		return
	}
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", "123", ""); e != ErrVerifyCodeCheckTimesUsedup {
		if e == nil {
			t.Fatal("should not pass")
		} else {
			t.Fatal(e)
		}
		return
	}
	if e := client.HasCheckTimes(context.Background(), "testuserid", "testaction"); e != ErrVerifyCodeCheckTimesUsedup {
		t.Fatal("should be check times used up")
		return
	}
	if e := client.CheckVerifyCode(context.Background(), "testuserid", "testaction", "123", ""); e != ErrVerifyCodeCheckTimesUsedup {
		if e == nil {
			t.Fatal("should not pass")
		} else {
			t.Fatal(e)
		}
		return
	}
	if _, _, e = client.MakeVerifyCode(context.Background(), "testuserid", "testaction", "112838923", 300); e != ErrVerifyCodeCheckTimesUsedup {
		t.Fatal("should be check times used up")
		return
	}
}
