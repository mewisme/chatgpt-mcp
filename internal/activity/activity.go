package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Event struct { Kind string `json:"kind"`; Tool string `json:"tool,omitempty"`; Message string `json:"message,omitempty"` }

type Store struct { Root string }

func (s Store) Append(ws workspace.Workspace, event Event) error {
	path := filepath.Join(s.Root, "workspaces", ws.ID, "activity.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return err }
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { return err }
	defer f.Close()
	data, _ := json.Marshal(event)
	_, err = f.Write(append(data, '\n'))
	return err
}
