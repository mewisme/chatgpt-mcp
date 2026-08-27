package mcp

import (
	"context"
	"encoding/json"

	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type ToolRuntime struct{ Runtime *tools.Runtime }

func NewToolRuntime(runtime *tools.Runtime) *ToolRuntime { return &ToolRuntime{Runtime: runtime} }

func (t *ToolRuntime) ListTools() any { return map[string]any{"tools": t.Runtime.List()} }

func (t *ToolRuntime) Call(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	args := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, err
		}
	}
	return t.Runtime.Call(ctx, name, args)
}
