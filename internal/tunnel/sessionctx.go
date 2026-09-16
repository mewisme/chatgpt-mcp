package tunnel

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

type sessionIDKey struct{}
type rpcRequestIDKey struct{}
type controlPlaneCommandIDKey struct{}
type requestIDKey struct{}

func ContextWithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

func SessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey{}).(string)
	return id, ok && id != ""
}

func ContextWithRPCRequestID(ctx context.Context, id jsonrpc.ID) context.Context {
	return context.WithValue(ctx, rpcRequestIDKey{}, id)
}

func RPCRequestIDFromContext(ctx context.Context) (jsonrpc.ID, bool) {
	id, ok := ctx.Value(rpcRequestIDKey{}).(jsonrpc.ID)
	return id, ok
}

func ContextWithControlPlaneCommandRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, controlPlaneCommandIDKey{}, id)
}

func ControlPlaneCommandRequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(controlPlaneCommandIDKey{}).(string)
	return id, ok && id != ""
}

func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)
	return id, ok && id != ""
}
