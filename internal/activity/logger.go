package activity

import "go.mewis.me/chatgpt-mcp/internal/workspace"

type Logger struct {
	Store  Store
	Stream *Stream
}

func NewLogger(root string) *Logger { return &Logger{Store: Store{Root: root}, Stream: NewStream()} }

func (l *Logger) Publish(ws workspace.Workspace, event Event) error {
	event = normalizeEvent(event)
	if err := l.Store.Append(ws, event); err != nil {
		return err
	}
	l.Stream.Publish(event)
	return nil
}
