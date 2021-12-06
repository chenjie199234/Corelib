package trace

import (
	"context"
	"testing"
	"time"
)

func Test_Trace(t *testing.T) {
	ctx := InitTrace(context.Background(), "", "fromapp", "fromip", "frommethod", "frompath", 0)
	now := time.Now()
	before := now.Add(-time.Minute)
	after := now.Add(time.Minute)
	Trace(ctx, CLIENT, "toapp", "toip", "tomethod", "topath", &before, &after, context.Canceled)
}
