package tools

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func callCoreTool(t *testing.T, registry *Registry, name string, args map[string]any) any {
	t.Helper()
	value, found, err := registry.Call(name, args)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	if !found {
		t.Fatalf("%s not registered", name)
	}
	return value
}

func TestCoreFileTools(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	workdir := t.TempDir()

	value := callCoreTool(t, registry, "write_file", map[string]any{"working_directory": workdir, "path": "nested/example.txt", "content": "hello"})
	written, ok := value.(WriteFileResult)
	if !ok {
		t.Fatalf("write_file returned %T", value)
	}
	if written.Bytes != 5 || written.Path != filepath.Join(workdir, "nested", "example.txt") {
		t.Fatalf("unexpected write result: %#v", written)
	}

	value = callCoreTool(t, registry, "read_text_file", map[string]any{"working_directory": workdir, "path": "nested/example.txt"})
	if content, ok := value.(string); !ok || content != "hello" {
		t.Fatalf("unexpected read_text_file result: %#v", value)
	}

	if err := os.WriteFile(filepath.Join(workdir, "second.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	value = callCoreTool(t, registry, "read_files", map[string]any{"working_directory": workdir, "paths": []any{"nested/example.txt", "second.txt"}})
	files, ok := value.([]ReadFile)
	if !ok || len(files) != 2 || files[0].Content != "hello" || files[1].Content != "world" {
		t.Fatalf("unexpected read_files result: %#v", value)
	}
}

func TestCoreRunCommand(t *testing.T) {
	registry := NewRegistry()
	RegisterCore(registry)
	workdir := t.TempDir()

	value := callCoreTool(t, registry, "run_command", map[string]any{"working_directory": workdir, "command": "echo chatgpt-mcp-tool"})
	result, ok := value.(CommandResult)
	if !ok {
		t.Fatalf("run_command returned %T", value)
	}
	if !result.Success || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "chatgpt-mcp-tool" {
		t.Fatalf("unexpected command result: %#v", result)
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

	value := callCoreTool(t, registry, "git_status", map[string]any{"working_directory": workdir})
	status, ok := value.(string)
	if !ok || !strings.Contains(status, "untracked.txt") {
		t.Fatalf("unexpected git_status result: %#v", value)
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
	}
}
