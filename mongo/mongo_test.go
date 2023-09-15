package mongo

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Mongo(t *testing.T) {
	c := &Config{
		MongoName:       "test",
		Addrs:           []string{"127.0.0.1:27017"},
		ReplicaSet:      "",
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second),
		IOTimeout:       ctime.Duration(time.Second),
	}
	_, e := NewMongo(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
