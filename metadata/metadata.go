package metadata

import (
	"context"

	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/web"
)

type metadatakey struct{}

func GetMetadata(ctx context.Context) map[string]string {
	if c, ok := ctx.(*crpc.Context); ok {
		return c.GetMetadata()
	} else if cc, ok := ctx.(*web.Context); ok {
		return cc.GetMetadata()
	} else if md, ok := ctx.Value(metadatakey{}).(map[string]string); ok {
		return md
	}
	return nil
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
