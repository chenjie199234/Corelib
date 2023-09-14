package mongo

import (
	"context"
	"testing"

	greadpref "go.mongodb.org/mongo-driver/mongo/readpref"
)

func Test_Mongo(t *testing.T) {
	c := &Config{}
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
