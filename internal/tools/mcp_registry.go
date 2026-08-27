package tools

import "context"

func CallMCP(ctx context.Context, client interface {
	Call(context.Context, string, string, map[string]any) (any, error)
}, server, tool string, args map[string]any) (any, error) {
	return client.Call(ctx, server, tool, args)
}
