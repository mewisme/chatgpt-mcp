//go:build !windows

package install

import (
	"os"
	"path/filepath"
)

func DefaultLayout() (Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, err
	}
	return defaultLayout(home)
}

func defaultLayout(home string) (Layout, error) {
	return NewLayout(filepath.Join(home, ".chatgpt-mcp"), filepath.Join(home, ".local", "bin"))
}

func platformBinaryNames() (string, string) {
	return "chatgpt-mcp", "cgm"
}
