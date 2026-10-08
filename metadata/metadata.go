package metadata

import (
	"context"
	"maps"
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
	if ctx == nil {
		if md == nil {
			return context.WithValue(context.Background(), metadatakey{}, map[string]string{})
		}
		return context.WithValue(context.Background(), metadatakey{}, md)
	}
	if existmd := GetMetadata(ctx); existmd != nil {
		for k := range existmd {
			delete(existmd, k)
		}
		maps.Copy(existmd, md)
		return ctx
	}
	if md == nil {
		return context.WithValue(ctx, metadatakey{}, map[string]string{})
	}
	//copy
	tmp := make(map[string]string, len(md))
	maps.Copy(tmp, md)
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
		maps.Copy(dstmd, srcmd)
		return dst, true
	}
	tmp := make(map[string]string, len(srcmd))
	maps.Copy(tmp, srcmd)
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

// one specific k-v pair in metadata which will be used in this lib
func AddToken(ctx context.Context, token string) context.Context {
	return AddMetadata(ctx, "Token", token)
}

// one specific k-v pair in medatada which will be used in this lib
func AddSession(ctx context.Context, session string) context.Context {
	return AddMetadata(ctx, "Session", session)
}

// one specific k-v pair in metadata which will be used in this lib
func AddAccessKey(ctx context.Context, accesskey string) context.Context {
	return AddMetadata(ctx, "Access-Key", accesskey)
}

func DelMetadata(ctx context.Context, key string) {
	md := GetMetadata(ctx)
	if md != nil {
		delete(md, key)
	}
}

// one specific key in metadata which will be used in this lib
func DelToken(ctx context.Context) {
	DelMetadata(ctx, "Token")
}

// one specific key in metadata which will be used in this lib
func DelSession(ctx context.Context) {
	DelMetadata(ctx, "Session")
}

// one specific key in metadata which will be used in this lib
func DelAccessKey(ctx context.Context) {
	DelMetadata(ctx, "Access-Key")
}

func HasMetadata(ctx context.Context, key string) bool {
	md := GetMetadata(ctx)
	var ok bool
	if md != nil {
		_, ok = md[key]
	}
	return ok
}

// one specific key in metadata which will be used in this lib
func HasToken(ctx context.Context) bool {
	return HasMetadata(ctx, "Token")
}

// one specific key in metadata which will be used in this lib
func HasSession(ctx context.Context) bool {
	return HasMetadata(ctx, "Session")
}

// one specific key in metadata which will be used in this lib
func HasAccessKey(ctx context.Context) bool {
	return HasMetadata(ctx, "Access-Key")
}
