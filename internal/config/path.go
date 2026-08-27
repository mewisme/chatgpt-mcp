package config

import (
	"os"
	"path/filepath"
)

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".config", "chatgpt-mcp", "config.json")
}
