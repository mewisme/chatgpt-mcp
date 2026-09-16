package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

type Layout struct {
	Scope         PluginScope
	ConfigRoot    string
	DataRoot      string
	CacheRoot     string
	WorkspaceRoot string
}

func DefaultLayout() Layout {
	configRoot := filepath.Clean(configformat.RootPath())
	defaultConfigRoot := filepath.Clean(configformat.DefaultRootPath())
	if configRoot != defaultConfigRoot {
		return Layout{Scope: ScopeGlobal, ConfigRoot: configRoot, DataRoot: configRoot + "-data", CacheRoot: configRoot + "-cache"}
	}
	return Layout{Scope: ScopeGlobal, ConfigRoot: configRoot, DataRoot: defaultDataRoot(), CacheRoot: defaultCacheRoot()}
}

func WorkspaceLayout(workspaceRoot string) (Layout, error) {
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	if workspaceRoot == "" {
		return Layout{}, errors.New("workspace root is required")
	}
	if !filepath.IsAbs(workspaceRoot) {
		return Layout{}, errors.New("workspace root must be absolute")
	}
	plugins := workspacestate.New(workspaceRoot).PluginsRoot()
	global := DefaultLayout()
	layout := Layout{
		Scope:         ScopeWorkspace,
		ConfigRoot:    plugins,
		DataRoot:      filepath.Join(plugins, "data"),
		CacheRoot:     global.CacheRoot,
		WorkspaceRoot: workspaceRoot,
	}
	if err := layout.Validate(); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func (layout Layout) EffectiveScope() PluginScope {
	if layout.Scope == "" {
		return ScopeGlobal
	}
	return layout.Scope
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
	switch layout.EffectiveScope() {
	case ScopeGlobal:
		if strings.TrimSpace(layout.WorkspaceRoot) != "" {
			return errors.New("global plugin layout cannot include a workspace root")
		}
	case ScopeWorkspace:
		if strings.TrimSpace(layout.WorkspaceRoot) == "" {
			return errors.New("workspace plugin layout requires a workspace root")
		}
		if !filepath.IsAbs(layout.WorkspaceRoot) {
			return errors.New("workspace root must be absolute")
		}
		cgm := workspacestate.New(layout.WorkspaceRoot).Root()
		if !pathWithin(cgm, layout.ConfigRoot) {
			return errors.New("workspace plugin config root must stay under .cgm")
		}
		if !pathWithin(cgm, layout.DataRoot) {
			return errors.New("workspace plugin data root must stay under .cgm")
		}
	default:
		return fmt.Errorf("unknown plugin scope: %q", layout.Scope)
	}
	return nil
}

func (layout Layout) ConfigPath() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return filepath.Join(layout.ConfigRoot, "desired.json")
	}
	return filepath.Join(layout.ConfigRoot, "plugins.json")
}

func (layout Layout) LockPath() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return filepath.Join(layout.ConfigRoot, "lock.json")
	}
	return filepath.Join(layout.ConfigRoot, "plugins.lock.json")
}

func (layout Layout) MutationLockPath() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return filepath.Join(layout.ConfigRoot, "mutation.lock")
	}
	return filepath.Join(layout.ConfigRoot, "plugins.mutation.lock")
}

func (layout Layout) PluginsPath() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return layout.DataRoot
	}
	return filepath.Join(layout.DataRoot, "plugins")
}

func (layout Layout) DownloadsPath() string {
	return filepath.Join(layout.CacheRoot, "plugins", "downloads")
}

func (layout Layout) InstalledVersionPath(id PluginID, version Version) string {
	return filepath.Join(layout.PluginsPath(), string(id), string(version))
}

func (layout Layout) PluginConfigPath(id PluginID) string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return filepath.Join(layout.ConfigRoot, "config", string(id)+".json")
	}
	return filepath.Join(layout.ConfigRoot, "plugins", "config", string(id)+".json")
}

func WorkspacePluginConfigPath(workspaceRoot string, id PluginID) string {
	return filepath.Join(filepath.Clean(workspaceRoot), workspacestate.DirectoryName, "plugins", "config", string(id)+".json")
}

func (layout Layout) RulesRoot() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return workspacestate.New(layout.WorkspaceRoot).RulesRoot()
	}
	return filepath.Join(layout.ConfigRoot, "rules")
}

func (layout Layout) SkillsRoot() string {
	if layout.EffectiveScope() == ScopeWorkspace {
		return workspacestate.New(layout.WorkspaceRoot).SkillsRoot()
	}
	return filepath.Join(layout.ConfigRoot, "skills")
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

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}
