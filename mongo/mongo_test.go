package mongo

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Mongo(t *testing.T) {
	c := &Config{
		MongoName:       "test",
		SRVName:         "",
		Addrs:           []string{"127.0.0.1:27017"},
		ReplicaSet:      "",
		UserName:        "",
		Password:        "",
		AuthDB:          "",
		MaxOpen:         256,
		MaxConnIdletime: ctime.Duration(time.Minute * 5),
		DialTimeout:     ctime.Duration(time.Second * 5),
		IOTimeout:       ctime.Duration(time.Second * 5),
	}
	_, e := NewMongo(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
