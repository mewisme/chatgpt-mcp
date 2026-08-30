package testutil

import (
	"os"
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func IsolateConfigHome() (string, func(), error) {
	home, err := os.MkdirTemp("", "chatgpt-mcp-test-home-")
	if err != nil {
		return "", nil, err
	}
	previous := map[string]*string{}
	for _, key := range []string{"HOME", "USERPROFILE", configformat.EnvConfigDir} {
		if value, ok := os.LookupEnv(key); ok {
			copy := value
			previous[key] = &copy
		} else {
			previous[key] = nil
		}
	}
	if err := os.Setenv("HOME", home); err != nil {
		_ = os.RemoveAll(home)
		return "", nil, err
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		_ = os.RemoveAll(home)
		return "", nil, err
	}
	if err := os.Setenv(configformat.EnvConfigDir, ""); err != nil {
		_ = os.RemoveAll(home)
		return "", nil, err
	}
	_ = configformat.SetRootPath("")
	cleanup := func() {
		_ = configformat.SetRootPath("")
		for key, value := range previous {
			if value == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *value)
			}
		}
		_ = os.RemoveAll(home)
	}
	return filepath.Join(home, ".config", "chatgpt-mcp"), cleanup, nil
}
