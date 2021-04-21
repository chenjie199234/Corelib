package egroup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test_Egroup(t *testing.T) {
	g := GetGroup(context.Background())
	g.Go(func(ctx context.Context) error {
		t.Log(1)
		return errors.New("123")
	})
	g.Go(func(ctx context.Context) error {
		time.Sleep(time.Second)
		t.Log(2)
		return nil
	})
	if e := PutGroup(g); e != nil {
		t.Log(e)
	}
}
