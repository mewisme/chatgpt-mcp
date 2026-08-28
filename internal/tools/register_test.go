package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newToolTestRuntime(t *testing.T) (*Runtime, string, string) {
	t.Helper()
	root := t.TempDir()
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, manager)
	RegisterCore(registry, manager)
	return &Runtime{Registry: registry, Workspaces: manager}, item.ID, root
}

func TestWorkspaceRegisterTool(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, manager)
	root := t.TempDir()
	result, err := registry.Call(context.Background(), "workspace_register", map[string]any{"path": root})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := result.StructuredContent.(WorkspaceRegistrationResult)
	if !ok || value.WorkspaceID == "" || value.WorkspaceRoot == "" {
		t.Fatalf("unexpected structured content: %#v", result.StructuredContent)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected MCP content: %#v", result.Content)
	}
}

func TestCoreFileToolsRequireWorkspaceBinding(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	result, err := runtime.Call(context.Background(), "write_file", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"path":              "nested/example.txt",
		"content":           "hello",
	})
	if err != nil || result.IsError {
		t.Fatalf("write_file failed: result=%#v err=%v", result, err)
	}
	if _, ok := result.StructuredContent.(WriteFileResult); !ok {
		t.Fatalf("unexpected write_file structured content: %T", result.StructuredContent)
	}

	result, err = runtime.Call(context.Background(), "read_text_file", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"path":              "nested/example.txt",
	})
	if err != nil || result.IsError {
		t.Fatalf("read_text_file failed: result=%#v err=%v", result, err)
	}
	value := result.StructuredContent.(ReadTextFileResult)
	if value.Content != "hello" {
		t.Fatalf("content = %q", value.Content)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	result, err = runtime.Call(context.Background(), "write_file", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"path":              outside,
		"content":           "denied",
	})
	if err != nil {
		t.Fatalf("workspace path violation should be tool error, not protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("outside write was not rejected: %#v", result)
	}
}

func TestRunCommandMutationGuard(t *testing.T) {
	if os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	runtime, workspaceID, root := newToolTestRuntime(t)

	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"command":           "printf safe > file.txt; rm file.txt",
	})
	if err != nil || result.IsError {
		t.Fatalf("safe mutation failed: result=%#v err=%v", result, err)
	}

	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err = runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"command":           "cd child && rm file.txt",
	})
	if err != nil {
		t.Fatalf("guard denial should be tool error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "cwd change") {
		t.Fatalf("cwd-changing mutation was not denied: %#v", result)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	result, err = runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"command":           "rm " + outside,
	})
	if err != nil {
		t.Fatalf("guard denial should be tool error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("outside mutation was not denied: %#v", result)
	}
}

func TestGitStatusUsesBoundWorkingDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	runtime, workspaceID, root := newToolTestRuntime(t)
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "git_status", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
	})
	if err != nil || result.IsError {
		t.Fatalf("git_status failed: result=%#v err=%v", result, err)
	}
	value := result.StructuredContent.(GitStatusResult)
	if !strings.Contains(value.Output, "untracked.txt") {
		t.Fatalf("unexpected git_status output: %#v", value)
	}
}

func TestCoreSchemasUseNativeOutputSchemas(t *testing.T) {
	runtime, _, _ := newToolTestRuntime(t)
	for _, schema := range runtime.List() {
		if len(schema.OutputSchema) == 0 {
			t.Fatalf("%s has no output schema", schema.Name)
		}
		var output map[string]any
		if err := json.Unmarshal(schema.OutputSchema, &output); err != nil {
			t.Fatalf("%s invalid output schema: %v", schema.Name, err)
		}
		if output["type"] != "object" {
			t.Fatalf("%s output schema type = %#v, want object", schema.Name, output["type"])
		}
		if _, wrapped := output["properties"].(map[string]any)["ok"]; wrapped {
			t.Fatalf("%s unexpectedly uses Local Coder result envelope", schema.Name)
		}
	}
}

func TestAnnotationsAreFixed(t *testing.T) {
	edit := ToolAnnotations(RiskEdit)
	if edit["destructiveHint"] != false || edit["openWorldHint"] != false || edit["idempotentHint"] != true {
		t.Fatalf("edit annotations changed with environment: %#v", edit)
	}
	command := ToolAnnotations(RiskCommand)
	if command["idempotentHint"] != false || command["destructiveHint"] != false {
		t.Fatalf("command annotations = %#v", command)
	}
}

func TestRegistryRejectsDuplicateTools(t *testing.T) {
	registry := NewRegistry()
	handler := func(context.Context, map[string]any) (Result, error) { return Result{}, nil }
	schema := DefaultSchema("test", "test")
	if err := registry.Register("test", schema, handler); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("test", schema, handler); !errors.Is(err, ErrToolAlreadyRegistered) {
		t.Fatalf("error = %v, want ErrToolAlreadyRegistered", err)
	}
}

func TestRegistryReturnsToolNotFound(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Call(context.Background(), "missing", nil); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("error = %v, want ErrToolNotFound", err)
	}
}
