package metadata

import (
	"context"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/grpc"
	"github.com/chenjie199234/Corelib/web"
)

type metadatakey struct{}

func GetMetadata(ctx context.Context) map[string]string {
	if wc, ok := ctx.(*web.Context); ok {
		return wc.GetMetadata()
	} else if cc, ok := ctx.(*crpc.Context); ok {
		return cc.GetMetadata()
	} else if gc, ok := ctx.(*grpc.Context); ok {
		return gc.GetMetadata()
	} else if md, ok := ctx.Value(metadatakey{}).(map[string]string); ok {
		return md
	}
	return nil
}
func SetMetadata(ctx context.Context, metadata map[string]string) context.Context {
	return context.WithValue(ctx, metadatakey{}, metadata)
}
func CopyMetadata(src context.Context) context.Context {
	if src == nil {
		return context.Background()
	}
	md := GetMetadata(src)
	if md == nil {
		return context.Background()
	}
	return context.WithValue(context.Background(), metadatakey{}, md)
}
