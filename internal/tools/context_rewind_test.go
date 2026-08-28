package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newContextToolRuntime(t *testing.T) (*Runtime, string, string, *checkpoint.Store) {
	t.Helper()
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "state"))
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, workspaces)
	RegisterContextTools(registry, workspaces, checkpoints)
	RegisterRewindTools(registry, workspaces, checkpoints)
	return &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}, item.ID, root, checkpoints
}

func TestContextSkillsRulesAndRemember(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, ".agents", "skills", "test")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test\ndescription: test skill\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	ruleDir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "ts.mdc"), []byte("---\nglobs: [\"**/*.ts\"]\n---\nTS rule"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxResult, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID})
	if err != nil || ctxResult.IsError {
		t.Fatalf("project_context failed: %#v %v", ctxResult, err)
	}
	if ctxResult.StructuredContent == nil {
		t.Fatal("missing project context")
	}

	listResult, err := runtime.Call(context.Background(), "list_skills", map[string]any{"workspace_id": workspaceID})
	if err != nil || listResult.IsError {
		t.Fatalf("list_skills failed: %#v %v", listResult, err)
	}
	if listResult.StructuredContent.(SkillsListResult).Count != 1 {
		t.Fatalf("skills = %#v", listResult.StructuredContent)
	}

	loadResult, err := runtime.Call(context.Background(), "load_skill", map[string]any{"workspace_id": workspaceID, "name": "test"})
	if err != nil || loadResult.IsError || !strings.Contains(loadResult.Content[0].Text, "body") {
		t.Fatalf("load_skill failed: %#v %v", loadResult, err)
	}

	rulesResult, err := runtime.Call(context.Background(), "load_path_rules", map[string]any{"workspace_id": workspaceID, "path": "src/app.ts"})
	if err != nil || rulesResult.IsError {
		t.Fatalf("load_path_rules failed: %#v %v", rulesResult, err)
	}
	if rulesResult.StructuredContent.(PathRulesResult).Count != 1 {
		t.Fatalf("rules = %#v", rulesResult.StructuredContent)
	}

	rememberResult, err := runtime.Call(context.Background(), "remember", map[string]any{"workspace_id": workspaceID, "note": "use compact imports"})
	if err != nil || rememberResult.IsError {
		t.Fatalf("remember failed: %#v %v", rememberResult, err)
	}
}

func TestRewindPreviewAndRestore(t *testing.T) {
	runtime, workspaceID, root, checkpoints := newContextToolRuntime(t)
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := checkpoints.Before(workspaceID, root, "edit_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}

	preview, err := runtime.Call(context.Background(), "rewind", map[string]any{"workspace_id": workspaceID, "action": "preview", "checkpoint_id": id})
	if err != nil || preview.IsError {
		t.Fatalf("preview failed: %#v %v", preview, err)
	}

	restore, err := runtime.Call(context.Background(), "rewind", map[string]any{"workspace_id": workspaceID, "action": "restore", "checkpoint_id": id})
	if err != nil || restore.IsError {
		t.Fatalf("restore failed: %#v %v", restore, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("content = %q", data)
	}
}
