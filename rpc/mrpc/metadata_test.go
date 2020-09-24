package mrpc

import (
	"context"
	"testing"
)

func Test_Ctx(t *testing.T) {
	ctx := context.Background()
	ctx = SetInMetadata(ctx, "in", "in")
	ctx = SetOutMetadata(ctx, "out", "out")
	if GetInMetadata(ctx, "in") != "in" {
		panic("missing")
	}
	if GetOutMetadata(ctx, "out") != "out" {
		panic("missing")
	}
}
