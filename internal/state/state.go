package state

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

func Root() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "chatgpt-mcp", "state")
}

func WorkspaceID(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])[:16]
}

func WorkspacePath(path string) string {
	return filepath.Join(Root(), "workspaces", WorkspaceID(path))
}
