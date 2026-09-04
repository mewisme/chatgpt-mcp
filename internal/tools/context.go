package tools

import (
	"context"
)

type InputRound struct {
	RequestState   string
	InputResponses map[string]any
}

type inputRoundContextKey struct{}
type approvalRequestContextKey struct{}

func WithInputRound(ctx context.Context, requestState string, inputResponses map[string]any) context.Context {
	if requestState == "" && inputResponses == nil {
		return ctx
	}
	return context.WithValue(ctx, inputRoundContextKey{}, InputRound{RequestState: requestState, InputResponses: inputResponses})
}

func InputRoundFromContext(ctx context.Context) InputRound {
	value, _ := ctx.Value(inputRoundContextKey{}).(InputRound)
	return value
}

func WithApprovalRequest(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, approvalRequestContextKey{}, requestID)
}

func ApprovalRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(approvalRequestContextKey{}).(string)
	return value
}
