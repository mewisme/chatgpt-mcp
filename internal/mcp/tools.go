package mcp

import "go.mewis.me/chatgpt-mcp/internal/tools"

type ToolRuntime struct{ Registry *tools.Registry }

func (t ToolRuntime) ListTools() []string {
	return t.Registry.List()
}

func (t ToolRuntime) Call(name string, args map[string]any) (any, error) {
	value, ok, err := t.Registry.Call(name, args)
	if err != nil {
		return nil, err
	}
	if !ok {
		return map[string]any{"error": "tool not found"}, nil
	}
	return value, nil
}
