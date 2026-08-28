package mcp

import "go.mewis.me/chatgpt-mcp/internal/activity"

func (h *HTTPRuntime) PublishToolsChanged() {
	if h != nil {
		PublishToolsChanged(h.Activity)
	}
}

func PublishToolsChanged(stream *activity.Stream) {
	if stream != nil {
		stream.Publish(activity.Event{Kind: "mcp.tools.changed", Message: "tools/list cache invalidated"})
	}
}
