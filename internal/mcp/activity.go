package mcp

import (
	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type ActivityLogger struct{ Store *activity.Store }

func (a ActivityLogger) ToolCall(workdir, tool string, result any) {
	if a.Store == nil {
		return
	}
	ws := workspace.Workspace{ID: workdir}
	_ = a.Store.Append(ws, activity.Event{Kind: "tool", Tool: tool, Message: "tool call completed"})
}
