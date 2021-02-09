package rpc

import (
	"context"
)

type metadatakey struct{}
type peernamekey struct{}
type pathkey struct{}
type deadlinekey struct{}

func GetAllMetadata(ctx context.Context) map[string]string {
	value := ctx.Value(metadatakey{})
	if value == nil {
		return nil
	}
	result, ok := value.(map[string]string)
	if !ok {
		return nil
	}
	return result
}
func GetMetadata(ctx context.Context, key string) string {
	value := ctx.Value(metadatakey{})
	if value == nil {
		return ""
	}
	result, ok := value.(map[string]string)
	if !ok {
		return ""
	}
	return result[key]
}
func SetMetadata(ctx context.Context, key, value string) context.Context {
	tempresult := ctx.Value(metadatakey{})
	if tempresult == nil {
		return context.WithValue(ctx, metadatakey{}, map[string]string{key: value})
	}
	result := tempresult.(map[string]string)
	result[key] = value
	return ctx
}
func SetAllMetadata(ctx context.Context, data map[string]string) context.Context {
	tempresult := ctx.Value(metadatakey{})
	if tempresult == nil {
		return context.WithValue(ctx, metadatakey{}, data)
	}
	result := tempresult.(map[string]string)
	for k, v := range data {
		result[k] = v
	}
	return ctx
}
func GetPath(ctx context.Context) string {
	value := ctx.Value(pathkey{})
	if value == nil {
		return ""
	}
	result, ok := value.(string)
	if !ok {
		return ""
	}
	return result
}
func SetPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, pathkey{}, path)
}
func GetPeerName(ctx context.Context) string {
	value := ctx.Value(peernamekey{})
	if value == nil {
		return ""
	}
	result, ok := value.(string)
	if !ok {
		return ""
	}
	return result
}
func SetPeerName(ctx context.Context, peername string) context.Context {
	return context.WithValue(ctx, peernamekey{}, peername)
}
func GetDeadline(ctx context.Context) int64 {
	value := ctx.Value(deadlinekey{})
	if value == nil {
		return 0
	}
	result, ok := value.(int64)
	if !ok {
		return 0
	}
	return result
}
func SetDeadline(ctx context.Context, dl int64) context.Context {
	return context.WithValue(ctx, deadlinekey{}, dl)
}
