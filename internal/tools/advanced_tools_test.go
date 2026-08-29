package tools

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newAdvancedRuntime(t *testing.T) (*Runtime, string, string) {
	t.Helper()
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, workspaces)
	RegisterAdvancedTools(registry, workspaces)
	return &Runtime{Registry: registry, Workspaces: workspaces}, item.ID, item.Path
}

func TestAdvancedToolCatalog(t *testing.T) {
	runtime, _, _ := newAdvancedRuntime(t)
	names := map[string]bool{}
	for _, schema := range runtime.List() {
		names[schema.Name] = true
	}
	for _, name := range []string{"node_repl", "ponytail_turn"} {
		if !names[name] {
			t.Fatalf("missing tool %q", name)
		}
	}
}

func TestNodeReplToolPersistsState(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	help, _ := exec.Command(node, "--help").CombinedOutput()
	if !strings.Contains(string(help), "--permission") && !strings.Contains(string(help), "--experimental-permission") {
		t.Skip("node permission model unavailable")
	}
	runtime, workspaceID, _ := newAdvancedRuntime(t)
	t.Cleanup(func() {
		result, err := runtime.Call(context.Background(), "node_repl", map[string]any{"workspace_id": workspaceID, "action": "reset"})
		if err != nil || result.IsError {
			t.Errorf("node_repl cleanup failed: result=%#v err=%v", result, err)
		}
	})
	first, err := runtime.Call(context.Background(), "node_repl", map[string]any{
		"workspace_id": workspaceID, "code": "globalThis.x = 1; return globalThis.x",
	})
	if err != nil || first.IsError {
		t.Fatalf("first eval failed: %#v %v", first, err)
	}
	second, err := runtime.Call(context.Background(), "node_repl", map[string]any{
		"workspace_id": workspaceID, "code": "globalThis.x += 1; return globalThis.x",
	})
	if err != nil || second.IsError {
		t.Fatalf("second eval failed: %#v %v", second, err)
	}
}
