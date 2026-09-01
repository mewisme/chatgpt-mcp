package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Event struct {
	Sequence    uint64         `json:"sequence,omitempty"`
	Kind        string         `json:"kind"`
	Method      string         `json:"method,omitempty"`
	Source      string         `json:"source,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	Status      string         `json:"status,omitempty"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
	Message     string         `json:"message,omitempty"`
	Raw         map[string]any `json:"raw,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

type Store struct{ Root string }

func (s Store) Append(ws workspace.Workspace, event Event) error {
	event = normalizeEvent(event)
	path := filepath.Join(s.Root, "workspaces", ws.ID, "activity.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(event)
	_, err = f.Write(append(data, '\n'))
	return err
}

func normalizeEvent(event Event) Event {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	return event
}
