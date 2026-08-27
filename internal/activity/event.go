package activity

type Type string

const (
	EventToolCall Type = "tool_call"
	EventSession  Type = "session"
	EventSystem   Type = "system"
)
