package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newShellToolTestRuntime(t *testing.T) (*Runtime, string, string) {
	t.Helper()
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "checkpoint-state"))
	registry := NewRegistry()
	shell := shellruntime.NewManager(workspaces, filepath.Join(t.TempDir(), "shell-state"))
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterFilesystemTools(registry, workspaces, checkpoints)
	processes := shellruntime.NewProcessManager(workspaces, shell)
	RegisterShellTools(registry, workspaces, shell, processes)
	return &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}, item.ID, item.Path
}

func TestShellToolsPersistCWD(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID,
		"command":      "cd child",
	})
	if err != nil || result.IsError {
		t.Fatalf("run_command failed: result=%#v err=%v", result, err)
	}
	statusResult, err := runtime.Call(context.Background(), "shell_status", map[string]any{"workspace_id": workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	status := statusResult.StructuredContent.(shellruntime.Status)
	if filepath.Clean(status.CWD) != filepath.Clean(child) {
		t.Fatalf("cwd = %q, want %q", status.CWD, child)
	}
	workspaceStatusResult, err := runtime.Call(context.Background(), "workspace_status", map[string]any{"workspace_id": workspaceID})
	if err != nil || workspaceStatusResult.IsError {
		t.Fatalf("workspace_status failed: result=%#v err=%v", workspaceStatusResult, err)
	}
	workspaceStatus := workspaceStatusResult.StructuredContent.(WorkspaceStatusResult)
	if filepath.Clean(workspaceStatus.WorkspaceRoot) != filepath.Clean(root) || filepath.Clean(workspaceStatus.ShellCWD) != filepath.Clean(child) {
		t.Fatalf("workspace status = %#v", workspaceStatus)
	}
	if len(workspaceStatus.AllowedDirectories) == 0 || filepath.Clean(workspaceStatus.AllowedDirectories[0]) != filepath.Clean(root) {
		t.Fatalf("allowed directories = %#v", workspaceStatus.AllowedDirectories)
	}
}

func TestShellMutationUsesPersistentCWD(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(child, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "cd child",
	})
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "rm file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("mutation failed: %#v", result)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestShellMutationRejectsCWDDirective(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "cd child && rm file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "cwd change") {
		t.Fatalf("mutation was not denied: %#v", result)
	}
}

func backgroundLifecycleCommand() string {
	if os.PathSeparator == '\\' {
		return "Write-Output ready; Start-Sleep -Milliseconds 100"
	}
	return "printf ready; sleep 0.1"
}

func TestBackgroundProcessLifecycle(t *testing.T) {
	if os.PathSeparator != '\\' && os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	runtime, workspaceID, _ := newShellToolTestRuntime(t)
	startResult, err := runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id": workspaceID,
		"command":      backgroundLifecycleCommand(),
	})
	if err != nil || startResult.IsError {
		t.Fatalf("start_process failed: result=%#v err=%v", startResult, err)
	}
	started := startResult.StructuredContent.(shellruntime.StartResult)
	if started.ID == "" || started.PID <= 0 {
		t.Fatalf("bad process result: %#v", started)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statusResult, err := runtime.Call(context.Background(), "process_status", map[string]any{"workspace_id": workspaceID, "id": started.ID})
		if err != nil {
			t.Fatal(err)
		}
		status := statusResult.StructuredContent.(ProcessStatusResult)
		if len(status.Processes) == 1 && !status.Processes[0].Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	outputResult, err := runtime.Call(context.Background(), "process_output", map[string]any{"workspace_id": workspaceID, "id": started.ID})
	if err != nil {
		t.Fatal(err)
	}
	output := outputResult.StructuredContent.(shellruntime.OutputResult)
	if !strings.Contains(output.Stdout, "ready") {
		t.Fatalf("stdout = %q", output.Stdout)
	}

	clearResult, err := runtime.Call(context.Background(), "clear_processes", map[string]any{"workspace_id": workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if clearResult.StructuredContent.(ClearProcessesResult).Cleared != 1 {
		t.Fatalf("clear result = %#v", clearResult.StructuredContent)
	}
}

func TestBackgroundMutationUsesPersistedCWDAndRejectsOutside(t *testing.T) {
	runtime, workspaceID, _ := newShellToolTestRuntime(t)
	result, err := runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id": workspaceID,
		"command":      "touch file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("background mutation failed: %#v", result)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	result, err = runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id": workspaceID,
		"command":      "rm " + outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("outside background mutation was not denied: %#v", result)
	}
}

func TestShellSchemasDoNotExposeWorkingDirectory(t *testing.T) {
	runtime, _, _ := newShellToolTestRuntime(t)
	for _, schema := range runtime.List() {
		if (schema.Name == "run_command" || schema.Name == "start_process") && strings.Contains(string(schema.InputSchema), `"working_directory"`) {
			t.Fatalf("%s still exposes working_directory: %s", schema.Name, schema.InputSchema)
		}
	}
}
