package tools

import "context"

type CallObservation struct {
	Phase                string
	Source               string
	Tool                 string
	WorkspaceID          string
	Status               string
	DurationMS           int64
	Message              string
	ResultType           string
	Raw                  map[string]any
	ReceivedByInstanceID string
	ExecutedByInstanceID string
}

type CallObserver func(CallObservation)

type callSourceKey struct{}
type callDetailsKey struct{}
type receivedByInstanceKey struct{}

type callDetails struct {
	Method  string
	Params  map[string]any
	Request map[string]any
}

func WithReceivedByInstanceID(ctx context.Context, instanceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, receivedByInstanceKey{}, instanceID)
}

func ReceivedByInstanceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(receivedByInstanceKey{}).(string)
	return value
}

func WithCallSource(ctx context.Context, source string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callSourceKey{}, source)
}

func CallSource(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	source, _ := ctx.Value(callSourceKey{}).(string)
	return source
}

func WithCallDetails(ctx context.Context, method string, params map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	details, _ := ctx.Value(callDetailsKey{}).(callDetails)
	details.Method = method
	details.Params = cloneMap(params)
	return context.WithValue(ctx, callDetailsKey{}, details)
}

func WithCallRequest(ctx context.Context, request map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	details, _ := ctx.Value(callDetailsKey{}).(callDetails)
	details.Request = cloneMap(request)
	return context.WithValue(ctx, callDetailsKey{}, details)
}

func callRaw(ctx context.Context, source, name string, args map[string]any) map[string]any {
	method := "tools/call"
	params := map[string]any{"name": name, "arguments": cloneMap(args)}
	if ctx != nil {
		if details, ok := ctx.Value(callDetailsKey{}).(callDetails); ok {
			if details.Method != "" {
				method = details.Method
			}
			if details.Params != nil {
				params = cloneMap(details.Params)
			}
			if details.Request != nil {
				request := cloneMap(details.Request)
				return map[string]any{"method": method, "source": source, "tool": name, "arguments": cloneMap(args), "params": params, "request": request}
			}
		}
	}
	return map[string]any{"method": method, "source": source, "tool": name, "arguments": cloneMap(args), "params": params}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = cloneValue(item)
	}
	return result
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = cloneValue(item)
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func (r *Runtime) SetCallObserver(observer CallObserver) {
	if r != nil {
		r.CallObserver = observer
	}
}

func (r *Runtime) HasCallObserver() bool {
	return r != nil && r.CallObserver != nil
}

func (r *Runtime) observeCall(observation CallObservation) {
	if r != nil && r.CallObserver != nil {
		r.CallObserver(observation)
	}
}
