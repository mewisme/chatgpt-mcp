package plugin

import (
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestLayoutPathsAreSeparated(t *testing.T) {
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	if err := layout.Validate(); err != nil {
		t.Fatal(err)
	}
	if layout.LockPath() == layout.InstalledVersionPath("bash", "1.0.0") || layout.DownloadsPath() == layout.PluginsPath() {
		t.Fatal("plugin roots overlap")
	}
}

func TestLayoutRejectsSharedRoots(t *testing.T) {
	root := t.TempDir()
	if err := (Layout{ConfigRoot: root, DataRoot: root, CacheRoot: filepath.Join(root, "cache")}).Validate(); err == nil {
		t.Fatal("shared config/data root accepted")
	}
}

func TestDefaultLayoutIsGlobal(t *testing.T) {
	layout := DefaultLayout()
	if layout.EffectiveScope() != ScopeGlobal {
		t.Fatalf("scope = %q", layout.EffectiveScope())
	}
	if filepath.Base(layout.ConfigPath()) != "plugins.json" || filepath.Base(layout.LockPath()) != "plugins.lock.json" {
		t.Fatalf("global paths = %s %s", layout.ConfigPath(), layout.LockPath())
	}
}

func TestWorkspaceLayoutUsesCgmPlugins(t *testing.T) {
	root := t.TempDir()
	layout, err := WorkspaceLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if layout.EffectiveScope() != ScopeWorkspace {
		t.Fatalf("scope = %q", layout.EffectiveScope())
	}
	plugins := filepath.Join(root, ".cgm", "plugins")
	if layout.ConfigPath() != filepath.Join(plugins, "desired.json") {
		t.Fatalf("desired = %s", layout.ConfigPath())
	}
	if layout.LockPath() != filepath.Join(plugins, "lock.json") {
		t.Fatalf("lock = %s", layout.LockPath())
	}
	if layout.PluginConfigPath("ponytail") != filepath.Join(plugins, "config", "ponytail.json") {
		t.Fatalf("config = %s", layout.PluginConfigPath("ponytail"))
	}
	if layout.InstalledVersionPath("bash", "1.0.0") != filepath.Join(plugins, "data", "bash", "1.0.0") {
		t.Fatalf("payload = %s", layout.InstalledVersionPath("bash", "1.0.0"))
	}
	if layout.DownloadsPath() != DefaultLayout().DownloadsPath() {
		t.Fatalf("workspace cache = %s", layout.DownloadsPath())
	}
	if layout.RulesRoot() != filepath.Join(root, ".cgm", "rules") {
		t.Fatalf("rules = %s", layout.RulesRoot())
	}
	if layout.SkillsRoot() != filepath.Join(root, ".cgm", "skills") {
		t.Fatalf("skills = %s", layout.SkillsRoot())
	}
	if strings.HasPrefix(layout.RulesRoot(), layout.ConfigRoot) || strings.HasPrefix(layout.SkillsRoot(), layout.ConfigRoot) {
		t.Fatal("workspace instruction roots leaked under plugins")
	}
}

func TestGlobalInstructionRootsFollowConfigRoot(t *testing.T) {
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	if layout.RulesRoot() != filepath.Join(root, "config", "rules") {
		t.Fatalf("rules = %s", layout.RulesRoot())
	}
	if layout.SkillsRoot() != filepath.Join(root, "config", "skills") {
		t.Fatalf("skills = %s", layout.SkillsRoot())
	}
}

func TestDefaultLayoutInstructionRootsFollowIsolatedConfigDir(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(configformat.EnvConfigDir, configDir)
	layout := DefaultLayout()
	if layout.RulesRoot() != filepath.Join(configDir, "rules") {
		t.Fatalf("rules = %s", layout.RulesRoot())
	}
	if layout.SkillsRoot() != filepath.Join(configDir, "skills") {
		t.Fatalf("skills = %s", layout.SkillsRoot())
	}
}

func TestWorkspaceLayoutRejectsEscape(t *testing.T) {
	root := t.TempDir()
	layout, err := WorkspaceLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	layout.ConfigRoot = t.TempDir()
	if err := layout.Validate(); err == nil {
		t.Fatal("escaped workspace config root accepted")
	}
	if _, err := WorkspaceLayout("relative"); err == nil {
		t.Fatal("relative workspace root accepted")
	}
}

func TestWorkspaceStoreInstallStaysUnderCgm(t *testing.T) {
	root := t.TempDir()
	layout, err := WorkspaceLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(testScopedManifest("bash", "1.0.0", "shell/bash", ScopeWorkspace), testPayload(t, "bash"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(installed.Root, filepath.Join(root, ".cgm", "plugins", "data")) {
		t.Fatalf("installed root = %s", installed.Root)
	}
	global := DefaultLayout()
	if strings.HasPrefix(installed.Root, global.PluginsPath()) {
		t.Fatalf("workspace install leaked into global plugins: %s", installed.Root)
	}
}
