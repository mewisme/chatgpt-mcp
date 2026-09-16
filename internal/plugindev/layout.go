package plugindev

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

func Layout(repoRoot, goos, goarch string) (plugin.Layout, error) {
	root, err := verifiedRoot(repoRoot)
	if err != nil {
		return plugin.Layout{}, fmt.Errorf("dev plugin layout: %w", err)
	}
	goos = strings.TrimSpace(goos)
	goarch = strings.TrimSpace(goarch)
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	platform := goos + "-" + goarch
	base := filepath.Join(root, ".cgm", "dev", "plugins", platform)
	layout := plugin.Layout{Scope: plugin.ScopeGlobal, ConfigRoot: base, DataRoot: filepath.Join(base, "data"), CacheRoot: filepath.Join(base, "cache")}
	if err := layout.Validate(); err != nil {
		return plugin.Layout{}, err
	}
	return layout, nil
}

func BootstrapLockPath(repoRoot string) (string, error) {
	root, err := verifiedRoot(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".cgm", "dev", "bootstrap.lock"), nil
}
