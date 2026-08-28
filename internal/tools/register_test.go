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
)

func callCoreTool(t *testing.T, registry *Registry, name string, args map[string]any) Result {
	t.Helper()
	result, err := registry.Call(context.Background(), name, args)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	return result
}

func TestCoreFileTools(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	workdir := t.TempDir()

	result := callCoreTool(t, registry, "write_file", map[string]any{"working_directory": workdir, "path": "nested/example.txt", "content": "hello"})
	written, ok := result.StructuredContent.(WriteFileResult)
	if !ok {
		t.Fatalf("write_file structured content returned %T", result.StructuredContent)
	}
	if written.Bytes != 5 || written.Path != filepath.Join(workdir, "nested", "example.txt") {
		t.Fatalf("unexpected write result: %#v", written)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected write envelope: %#v", result)
	}

	result = callCoreTool(t, registry, "read_text_file", map[string]any{"working_directory": workdir, "path": "nested/example.txt"})
	if content, ok := result.StructuredContent.(string); !ok || content != "hello" {
		t.Fatalf("unexpected read_text_file result: %#v", result)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Fatalf("unexpected read_text_file content: %#v", result.Content)
	}

	if err := os.WriteFile(filepath.Join(workdir, "second.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	result = callCoreTool(t, registry, "read_files", map[string]any{"working_directory": workdir, "paths": []any{"nested/example.txt", "second.txt"}})
	readFiles, ok := result.StructuredContent.([]ReadFile)
	if !ok || len(readFiles) != 2 || readFiles[0].Content != "hello" || readFiles[1].Content != "world" {
		t.Fatalf("unexpected read_files result: %#v", result)
	}
}

func TestRuntimeConvertsToolFailureToErrorResult(t *testing.T) {
	runtime := NewRuntime()
	result, err := runtime.Call(context.Background(), "read_text_file", map[string]any{"working_directory": t.TempDir(), "path": "missing.txt"})
	if err != nil {
		t.Fatalf("runtime returned protocol error for tool failure: %v", err)
	}
	if !result.IsError || len(result.Content) != 1 || result.Content[0].Text == "" {
		t.Fatalf("unexpected error result: %#v", result)
	}
	if result.StructuredContent != nil {
		t.Fatalf("error result must not expose structuredContent: %#v", result.StructuredContent)
	}
}

func TestCoreRunCommand(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	workdir := t.TempDir()

	result := callCoreTool(t, registry, "run_command", map[string]any{"working_directory": workdir, "command": "echo chatgpt-mcp-tool"})
	value, ok := result.StructuredContent.(CommandResult)
	if !ok {
		t.Fatalf("run_command structured content returned %T", result.StructuredContent)
	}
	if result.IsError || !value.Success || value.ExitCode != 0 || strings.TrimSpace(value.Stdout) != "chatgpt-mcp-tool" {
		t.Fatalf("unexpected command result: %#v / %#v", result, value)
	}
}

func TestCoreRunCommandNonZeroIsToolError(t *testing.T) {
	if os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	registry := NewRegistry()
	RegisterCore(registry)
	result := callCoreTool(t, registry, "run_command", map[string]any{"working_directory": t.TempDir(), "command": "exit 7"})
	value, ok := result.StructuredContent.(CommandResult)
	if !ok {
		t.Fatalf("run_command structured content returned %T", result.StructuredContent)
	}
	if !result.IsError || value.Success || value.ExitCode != 7 {
		t.Fatalf("unexpected non-zero command result: %#v / %#v", result, value)
	}
}

func TestCoreGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	registry := NewRegistry()
	RegisterCore(registry)
	workdir := t.TempDir()

	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = workdir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workdir, "untracked.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	result := callCoreTool(t, registry, "git_status", map[string]any{"working_directory": workdir})
	status, ok := result.StructuredContent.(string)
	if !ok || !strings.Contains(status, "untracked.txt") {
		t.Fatalf("unexpected git_status result: %#v", result)
	}
}

func TestCoreSchemasDescribeRequiredArguments(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	for _, schema := range registry.ListSchemas() {
		var input struct {
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(schema.InputSchema, &input); err != nil {
			t.Fatalf("%s has invalid input schema: %v", schema.Name, err)
		}
		found := false
		for _, key := range input.Required {
			if key == "working_directory" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s does not require working_directory", schema.Name)
		}
		if len(schema.OutputSchema) == 0 {
			t.Fatalf("%s does not define outputSchema", schema.Name)
		}
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

func TestRuntimePreservesToolNotFound(t *testing.T) {
	runtime := NewRuntime()
	if _, err := runtime.Call(context.Background(), "missing", nil); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("error = %v, want ErrToolNotFound", err)
	}
}

func TestRegistryPassesContext(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := registry.Register("context_test", DefaultSchema("context_test", "context test"), func(ctx context.Context, args map[string]any) (Result, error) {
		return Result{}, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Call(ctx, "context_test", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRegistrySchemasAreSorted(t *testing.T) {
	registry := NewRegistry()
	handler := func(context.Context, map[string]any) (Result, error) { return Result{}, nil }
	for _, name := range []string{"zeta", "alpha", "middle"} {
		if err := registry.Register(name, DefaultSchema(name, name), handler); err != nil {
			t.Fatal(err)
		}
	}
	schemas := registry.ListSchemas()
	for i, want := range []string{"alpha", "middle", "zeta"} {
		if schemas[i].Name != want {
			t.Fatalf("schema[%d] = %q, want %q", i, schemas[i].Name, want)
		}
	}
}
