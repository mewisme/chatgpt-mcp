package state

import (
	"os"
	"path/filepath"
)

func Root() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/chatgpt-mcp/state"
	}
	return filepath.Join(home, ".config", "chatgpt-mcp", "state")
}

func Workspace(id string) string {
	return filepath.Join(Root(), "workspaces", id)
}
