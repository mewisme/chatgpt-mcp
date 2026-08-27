package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

type Event struct {
	ID string `json:"id"`
	Time string `json:"time"`
	Kind string `json:"kind"`
	Tool string `json:"tool,omitempty"`
	Action string `json:"action,omitempty"`
	Status string `json:"status,omitempty"`
	Details any `json:"details,omitempty"`
}

func Append(workspace string, event Event) error {
	root := filepath.Join(state.Root(), "workspaces", state.WorkspaceID(workspace))
	if err := os.MkdirAll(root, 0700); err != nil { return err }
	path := filepath.Join(root, "activity.jsonl")
	if event.Time == "" { event.Time = time.Now().UTC().Format(time.RFC3339) }
	data, err := json.Marshal(event)
	if err != nil { return err }
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { return err }
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}
