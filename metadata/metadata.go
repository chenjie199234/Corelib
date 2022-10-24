package metadata

import (
	"context"

	"github.com/chenjie199234/Corelib/cgrpc"
	"github.com/chenjie199234/Corelib/crpc"
	"github.com/chenjie199234/Corelib/web"
)

type metadatakey struct{}

func GetMetadata(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	if wc, ok := ctx.(*web.Context); ok {
		return wc.GetMetadata()
	} else if cc, ok := ctx.(*crpc.Context); ok {
		return cc.GetMetadata()
	} else if gc, ok := ctx.(*cgrpc.Context); ok {
		return gc.GetMetadata()
	} else if md, ok := ctx.Value(metadatakey{}).(map[string]string); ok {
		return md
	}
	return nil
}
func SetMetadata(ctx context.Context, metadata map[string]string) context.Context {
	return context.WithValue(ctx, metadatakey{}, metadata)
}

// this will overwrite dst's metadata
func CopyMetadata(src, dst context.Context) (context.Context, bool) {
	if dst == nil {
		dst = context.Background()
	}
	if src == nil {
		return dst, false
	}
	md := GetMetadata(src)
	if md == nil {
		return dst, false
	}
	return context.WithValue(dst, metadatakey{}, md), true
}
func AddMetadata(ctx context.Context, key, value string) context.Context {
	md := GetMetadata(ctx)
	if md == nil {
		if ctx == nil {
			return context.WithValue(context.Background(), metadatakey{}, map[string]string{key: value})
		}
		return context.WithValue(ctx, metadatakey{}, map[string]string{key: value})
	}
	md[key] = value
	return ctx
}
func DelMetadata(ctx context.Context, key string) {
	md := GetMetadata(ctx)
	if md != nil {
		delete(md, key)
	}
}
