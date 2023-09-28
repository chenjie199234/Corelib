package metadata

import (
	"context"
)

type metadatakey struct{}

func GetMetadata(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	if md, ok := ctx.Value(metadatakey{}).(map[string]string); ok {
		return md
	}
	return nil
}
func SetMetadata(ctx context.Context, md map[string]string) context.Context {
	if md == nil {
		return context.WithValue(ctx, metadatakey{}, map[string]string{})
	}
	//copy
	tmp := make(map[string]string, len(md))
	for k, v := range md {
		tmp[k] = v
	}
	return context.WithValue(ctx, metadatakey{}, tmp)
}

// this will overwrite dst's metadata
// dst's and src's metadata are different map with same data
// if src doesn't have metadata,false will return and dst will not be changed
func CopyMetadata(dst, src context.Context) (context.Context, bool) {
	if dst == nil {
		dst = context.Background()
	}
	srcmd := GetMetadata(src)
	if srcmd == nil {
		return dst, false
	}
	dstmd := GetMetadata(dst)
	if dstmd != nil {
		//delete old
		for k := range dstmd {
			delete(dstmd, k)
		}
		//copy new
		for k, v := range srcmd {
			dstmd[k] = v
		}
		return dst, true
	}
	tmp := make(map[string]string, len(srcmd))
	for k, v := range srcmd {
		tmp[k] = v
	}
	return context.WithValue(dst, metadatakey{}, tmp), true
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
func HasMetadata(ctx context.Context, key string) bool {
	md := GetMetadata(ctx)
	var ok bool
	if md != nil {
		_, ok = md[key]
	}
	return ok
}
