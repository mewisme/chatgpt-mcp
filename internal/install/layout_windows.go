//go:build windows

package install

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultLayout() (Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, err
	}
	return defaultLayout(home, os.Getenv("LOCALAPPDATA"))
}

func defaultLayout(home, localAppData string) (Layout, error) {
	localAppData = strings.TrimSpace(localAppData)
	if localAppData == "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	root := filepath.Join(localAppData, "chatgpt-mcp")
	return NewLayout(root, filepath.Join(root, "current"))
}

func platformBinaryNames() (string, string) {
	return "chatgpt-mcp.exe", "cgm.cmd"
}
