package trace

import "context"

type tracekey struct{}

//trace
func GetTrace(ctx context.Context) string {
	value := ctx.Value(tracekey{})
	if value == nil {
		return ""
	}
	result, ok := value.(string)
	if !ok {
		return ""
	}
	return result
}
func SetTrace(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, tracekey{}, value)
}
