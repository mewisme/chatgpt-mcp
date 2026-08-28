package config

import (
	"os"
	"path/filepath"
)

func RootPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "chatgpt-mcp"
	}
	return filepath.Join(home, ".config", "chatgpt-mcp")
}

func DefaultPath() string { return filepath.Join(RootPath(), "config.json") }
