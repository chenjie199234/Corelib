package mongo

import (
	"context"
	"testing"
	"time"

	greadpref "go.mongodb.org/mongo-driver/mongo/readpref"
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
	db, e := NewMongo(c, nil)
	if e != nil {
		t.Fatal(e)
		return
	}
	if e = db.Ping(context.Background(), greadpref.Primary()); e != nil {
		t.Fatal(e)
		return
	}
	return
}
