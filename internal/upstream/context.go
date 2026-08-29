package upstream

import "context"

type requestMetaContextKey struct{}

func WithRequestMeta(ctx context.Context, meta map[string]any) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	return context.WithValue(ctx, requestMetaContextKey{}, cloneMap(meta))
}

func RequestMetaFromContext(ctx context.Context) map[string]any {
	value, _ := ctx.Value(requestMetaContextKey{}).(map[string]any)
	if len(value) == 0 {
		return nil
	}
	return cloneMap(value)
}
