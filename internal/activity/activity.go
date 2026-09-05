package activity

import (
	"time"
)

type Event struct {
	Sequence             uint64         `json:"sequence,omitempty"`
	CallID               string         `json:"call_id,omitempty"`
	Kind                 string         `json:"kind"`
	Method               string         `json:"method,omitempty"`
	Source               string         `json:"source,omitempty"`
	Tool                 string         `json:"tool,omitempty"`
	WorkspaceID          string         `json:"workspace_id,omitempty"`
	SessionHash          string         `json:"session_hash,omitempty"`
	SessionBinding       string         `json:"session_binding,omitempty"`
	SessionWorkspaceID   string         `json:"session_workspace_id,omitempty"`
	ReceivedByInstanceID string         `json:"received_by_instance_id,omitempty"`
	ExecutedByInstanceID string         `json:"executed_by_instance_id,omitempty"`
	Status               string         `json:"status,omitempty"`
	DurationMS           int64          `json:"duration_ms,omitempty"`
	Message              string         `json:"message,omitempty"`
	Raw                  map[string]any `json:"raw,omitempty"`
	Timestamp            time.Time      `json:"timestamp"`
}

func normalizeEvent(event Event) Event {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	return event
}
