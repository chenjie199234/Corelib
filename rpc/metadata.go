package rpc

import (
	"context"
)

type metadatakey struct{}

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
