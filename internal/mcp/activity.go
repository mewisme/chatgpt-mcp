package mcp

import "go.mewis.me/chatgpt-mcp/internal/activity"

func (h *HTTPRuntime) EmitActivity(kind, message string) {
	if h.Activity == nil {
		return
	}
	h.Activity.Publish(activity.Event{Kind: kind, Message: message})
}
