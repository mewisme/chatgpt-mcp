package mcp

import "go.mewis.me/chatgpt-mcp/internal/activity"

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func ToolsListChanged() Notification {
	return Notification{JSONRPC: "2.0", Method: "notifications/tools/list_changed"}
}

func PublishToolsChanged(stream *activity.Stream) {
	if stream != nil {
		stream.Publish(activity.Event{Kind: "mcp.notification", Message: "tools/list_changed"})
	}
}
