package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
