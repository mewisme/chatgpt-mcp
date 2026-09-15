package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

type Layout struct {
	ConfigRoot string
	DataRoot   string
	CacheRoot  string
}

func DefaultLayout() Layout {
	configRoot := filepath.Clean(configformat.RootPath())
	defaultConfigRoot := filepath.Clean(configformat.DefaultRootPath())
	if configRoot != defaultConfigRoot {
		return Layout{ConfigRoot: configRoot, DataRoot: configRoot + "-data", CacheRoot: configRoot + "-cache"}
	}
	return Layout{ConfigRoot: configRoot, DataRoot: defaultDataRoot(), CacheRoot: defaultCacheRoot()}
}

func (layout Layout) Validate() error {
	for name, value := range map[string]string{"config": layout.ConfigRoot, "data": layout.DataRoot, "cache": layout.CacheRoot} {
		if strings.TrimSpace(value) == "" {
			return errors.New(name + " root is required")
		}
		if !filepath.IsAbs(value) {
			return errors.New(name + " root must be absolute")
		}
	}
	if sameCleanPath(layout.ConfigRoot, layout.DataRoot) || sameCleanPath(layout.ConfigRoot, layout.CacheRoot) || sameCleanPath(layout.DataRoot, layout.CacheRoot) {
		return errors.New("plugin config, data, and cache roots must be distinct")
	}
	return nil
}

func (layout Layout) ConfigPath() string  { return filepath.Join(layout.ConfigRoot, "plugins.json") }
func (layout Layout) LockPath() string    { return filepath.Join(layout.ConfigRoot, "plugins.lock.json") }
func (layout Layout) PluginsPath() string { return filepath.Join(layout.DataRoot, "plugins") }
func (layout Layout) DownloadsPath() string {
	return filepath.Join(layout.CacheRoot, "plugins", "downloads")
}

func (layout Layout) InstalledVersionPath(id PluginID, version Version) string {
	return filepath.Join(layout.PluginsPath(), string(id), string(version))
}

func defaultDataRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Clean("chatgpt-mcp-data")
	}
	switch runtime.GOOS {
	case "windows":
		if value := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); value != "" {
			return filepath.Join(value, "chatgpt-mcp", "data")
		}
		return filepath.Join(home, "AppData", "Local", "chatgpt-mcp", "data")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "chatgpt-mcp")
	default:
		if value := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); value != "" {
			return filepath.Join(value, "chatgpt-mcp")
		}
		return filepath.Join(home, ".local", "share", "chatgpt-mcp")
	}
}

func defaultCacheRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Clean("chatgpt-mcp-cache")
	}
	switch runtime.GOOS {
	case "windows":
		if value := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); value != "" {
			return filepath.Join(value, "chatgpt-mcp", "cache")
		}
		return filepath.Join(home, "AppData", "Local", "chatgpt-mcp", "cache")
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "chatgpt-mcp")
	default:
		if value := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); value != "" {
			return filepath.Join(value, "chatgpt-mcp")
		}
		return filepath.Join(home, ".cache", "chatgpt-mcp")
	}
}

func sameCleanPath(left, right string) bool { return filepath.Clean(left) == filepath.Clean(right) }
