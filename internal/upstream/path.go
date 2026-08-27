package upstream

import (
	"os"
	"path/filepath"
)

func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "chatgpt-mcp", "upstream.json")
}
