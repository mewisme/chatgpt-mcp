package tools

import "context"

type CallObservation struct {
	Phase       string
	Source      string
	Tool        string
	WorkspaceID string
	Status      string
	DurationMS  int64
	Message     string
	ResultType  string
}

type CallObserver func(CallObservation)

type callSourceKey struct{}

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
