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

func (h *HTTPRuntime) PublishNotification(notification Notification) {
	if h == nil || h.Sessions == nil {
		return
	}
	for _, session := range h.Sessions.List() {
		session.Notify(notification)
	}
	h.EmitActivity("mcp.notification", notification.Method)
}

func (h *HTTPRuntime) PublishToolsChanged() { h.PublishNotification(ToolsListChanged()) }

func PublishToolsChanged(stream *activity.Stream) {
	if stream != nil {
		stream.Publish(activity.Event{Kind: "mcp.notification", Message: "tools/list_changed"})
	}
}
