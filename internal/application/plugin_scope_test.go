package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestResolvePluginLayoutDefaultsToGlobal(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	layout, err := ResolvePluginLayoutWith(PluginScopeOptions{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	if layout.EffectiveScope() != pluginpkg.ScopeGlobal || layout.WorkspaceRoot != "" {
		t.Fatalf("layout = %#v", layout)
	}
}

func TestResolvePluginLayoutWorkspaceFlagImpliesWorkspace(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ResolvePluginLayoutWith(PluginScopeOptions{Workspace: item.ID}, manager)
	if err != nil {
		t.Fatal(err)
	}
	if layout.EffectiveScope() != pluginpkg.ScopeWorkspace || layout.WorkspaceRoot != item.Path {
		t.Fatalf("layout = %#v want workspace %s", layout, item.Path)
	}
}

func TestResolvePluginLayoutRejectsGlobalWithWorkspace(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	_, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "global", Workspace: "ws_x"}, manager)
	if err == nil || !strings.Contains(err.Error(), "--scope global cannot be combined with --workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvePluginLayoutRejectsUnknownScope(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	_, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "cluster"}, manager)
	if err == nil || !strings.Contains(err.Error(), "plugin --scope must be global or workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvePluginLayoutUsesSingleRegisteredWorkspace(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "workspace"}, manager)
	if err != nil {
		t.Fatal(err)
	}
	if layout.WorkspaceRoot != item.Path {
		t.Fatalf("layout = %#v want %s", layout, item.Path)
	}
}

func TestResolvePluginLayoutRequiresWorkspaceWhenAmbiguous(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	if _, err := manager.Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	_, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "workspace"}, manager)
	if err == nil || !strings.Contains(err.Error(), "multiple workspaces; specify --workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvePluginLayoutRequiresWorkspaceWhenNoneRegistered(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	_, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "workspace"}, manager)
	if err == nil || !strings.Contains(err.Error(), "no registered workspace; specify --workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvePluginLayoutMatchesWorkspacePath(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := ResolvePluginLayoutWith(PluginScopeOptions{Scope: "workspace", Workspace: root}, manager)
	if err != nil {
		t.Fatal(err)
	}
	if layout.WorkspaceRoot != item.Path {
		t.Fatalf("layout = %#v want %s", layout, item.Path)
	}
}

func TestResolvePluginLayoutRejectsUnavailableWorkspace(t *testing.T) {
	store := filepath.Join(t.TempDir(), "workspaces.json")
	manager := workspace.NewManager(store)
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	_, err = ResolvePluginLayoutWith(PluginScopeOptions{Workspace: item.ID}, workspace.NewManager(store))
	if err == nil || !errors.Is(err, workspace.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestNewPluginServiceForLayoutUsesWorkspaceStore(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	root := t.TempDir()
	layout, err := pluginpkg.WorkspaceLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPluginServiceForLayout(layout)
	if err != nil {
		t.Fatal(err)
	}
	if service.Layout.EffectiveScope() != pluginpkg.ScopeWorkspace {
		t.Fatalf("layout = %#v", service.Layout)
	}
	items, err := service.Installed()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Origin == pluginpkg.OriginBuiltin {
			t.Fatalf("workspace plugin service included builtin %s", item.ID)
		}
	}
}

func TestApplyPluginInstallScope(t *testing.T) {
	explicit, err := ApplyPluginInstallScope(PluginScopeOptions{Scope: "global"}, []pluginpkg.PluginScope{pluginpkg.ScopeGlobal, pluginpkg.ScopeWorkspace})
	if err != nil || explicit.Scope != "global" {
		t.Fatalf("explicit = %#v %v", explicit, err)
	}
	implied, err := ApplyPluginInstallScope(PluginScopeOptions{Workspace: "ws_x"}, []pluginpkg.PluginScope{pluginpkg.ScopeGlobal, pluginpkg.ScopeWorkspace})
	if err != nil || implied.Workspace != "ws_x" {
		t.Fatalf("implied = %#v %v", implied, err)
	}
	single, err := ApplyPluginInstallScope(PluginScopeOptions{}, []pluginpkg.PluginScope{pluginpkg.ScopeWorkspace})
	if err != nil || single.Scope != string(pluginpkg.ScopeWorkspace) {
		t.Fatalf("single = %#v %v", single, err)
	}
	legacy, err := ApplyPluginInstallScope(PluginScopeOptions{}, nil)
	if err != nil || legacy.Scope != string(pluginpkg.ScopeGlobal) {
		t.Fatalf("legacy = %#v %v", legacy, err)
	}
	_, err = ApplyPluginInstallScope(PluginScopeOptions{}, []pluginpkg.PluginScope{pluginpkg.ScopeGlobal, pluginpkg.ScopeWorkspace})
	if !errors.Is(err, ErrPluginScopeRequired) {
		t.Fatalf("multi-scope error = %v", err)
	}
}

func TestAttachPluginPeersWiresGlobalAndWorkspaceStores(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	workspaces := workspace.NewManager(workspace.DefaultStorePath())
	item, err := workspaces.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	global, err := pluginpkg.NewStore(pluginpkg.DefaultLayout(), pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachPluginPeers(global); err != nil {
		t.Fatal(err)
	}
	if len(global.Peers()) != 1 || global.Peers()[0].Layout().WorkspaceRoot != item.Path {
		t.Fatalf("global peers = %#v", global.Peers())
	}
	layout, err := pluginpkg.WorkspaceLayout(item.Path)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachPluginPeers(scoped); err != nil {
		t.Fatal(err)
	}
	if len(scoped.Peers()) != 1 || scoped.Peers()[0].Layout().EffectiveScope() != pluginpkg.ScopeGlobal {
		t.Fatalf("workspace peers = %#v", scoped.Peers())
	}
}

func TestNewPluginServiceForOptionsUsesWorkspaceLayout(t *testing.T) {
	t.Setenv(configformat.EnvConfigDir, t.TempDir())
	item, err := workspace.NewManager(workspace.DefaultStorePath()).Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPluginServiceForOptions(PluginScopeOptions{Scope: "workspace", Workspace: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	if service.Layout.EffectiveScope() != pluginpkg.ScopeWorkspace || service.Workspace != item.ID {
		t.Fatalf("service = %#v", service)
	}
	items, err := service.Installed()
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range items {
		if listed.Origin == pluginpkg.OriginBuiltin {
			t.Fatalf("workspace service included builtin %s", listed.ID)
		}
		if listed.Scope != pluginpkg.ScopeWorkspace {
			t.Fatalf("listed scope = %#v", listed)
		}
	}
}
