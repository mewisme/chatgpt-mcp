package mcp

import "go.mewis.me/chatgpt-mcp/internal/activity"

type ActivityLogger struct{ Stream *activity.Stream }

func (a ActivityLogger) Emit(event activity.Event) {
	if a.Stream != nil {
		a.Stream.Publish(event)
	}
}
