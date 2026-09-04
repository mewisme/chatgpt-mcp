package controlguard

import (
	"context"
	"strings"
)

type approvalContextKey struct{}

type Approval struct {
	RequestID  string
	Capability string
	Invocation Invocation
}

func WithApproval(ctx context.Context, value Approval) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	value.RequestID = strings.TrimSpace(value.RequestID)
	value.Capability = strings.TrimSpace(value.Capability)
	value.Invocation = *cloneInvocation(&value.Invocation)
	if value.RequestID == "" || value.Capability == "" {
		return ctx
	}
	return context.WithValue(ctx, approvalContextKey{}, value)
}

func ApprovalFromContext(ctx context.Context) (Approval, bool) {
	if ctx == nil {
		return Approval{}, false
	}
	value, ok := ctx.Value(approvalContextKey{}).(Approval)
	if !ok || strings.TrimSpace(value.RequestID) == "" || strings.TrimSpace(value.Capability) == "" {
		return Approval{}, false
	}
	value.Invocation = *cloneInvocation(&value.Invocation)
	return value, true
}

func SameInvocation(left, right Invocation) bool {
	if strings.TrimSpace(left.Command) != strings.TrimSpace(right.Command) || strings.TrimSpace(left.Program) != strings.TrimSpace(right.Program) || len(left.Args) != len(right.Args) {
		return false
	}
	for index := range left.Args {
		if left.Args[index] != right.Args[index] {
			return false
		}
	}
	return true
}
