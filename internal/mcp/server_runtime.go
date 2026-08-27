package mcp

import (
	"context"

	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type Runtime struct {
	Tools *tools.Runtime
}

func NewRuntime() *Runtime {
	r := tools.NewRuntime()
	return &Runtime{Tools: r}
}

func (r *Runtime) Handle(ctx context.Context, method string, params map[string]any) (any, error) {
	switch method {
	case "initialize":
		return Initialize(params), nil
	case "notifications/initialized":
		return nil, nil
	case "tools/list":
		return map[string]any{"tools": r.Tools.List()}, nil
	case "tools/call":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		return r.Tools.Call(ctx, name, args)
	default:
		return nil, nil
	}
}
