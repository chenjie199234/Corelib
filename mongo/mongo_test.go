package mongo

import (
	"testing"
	"time"
)

func Test_Mongo(t *testing.T) {
	c := &Config{
		MongoName:       "test",
		Addrs:           []string{"127.0.0.1:27017"},
		ReplicaSet:      "",
		MaxOpen:         256,
		MaxConnIdleTime: time.Minute * 5,
		DialTimeout:     time.Second,
		IOTimeout:       time.Second,
	}
	_, e := NewMongo(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	return
}
