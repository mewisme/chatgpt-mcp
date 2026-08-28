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
	RegisterWorkspaceTools(registry, workspaces)
	RegisterFilesystemTools(registry, workspaces, checkpoints)
	shell := shellruntime.NewManager(workspaces, filepath.Join(t.TempDir(), "shell-state"))
	processes := shellruntime.NewProcessManager(workspaces, shell)
	RegisterShellTools(registry, workspaces, shell, processes)
	return &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}, item.ID, root
}

func TestShellToolsPersistCWD(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id":      workspaceID,
		"command":           "cd child",
		"working_directory": root,
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
}

func TestShellMutationRequiresMatchingCWD(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	_, _ = runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "cd child", "working_directory": root,
	})
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "rm file.txt", "working_directory": root,
	})
	if err != nil {
		t.Fatalf("expected MCP tool error result, got protocol error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "does not match working_directory") {
		t.Fatalf("mutation was not denied: %#v", result)
	}
}

func TestShellMutationRejectsCWDDirective(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "run_command", map[string]any{
		"workspace_id": workspaceID, "command": "cd child && rm file.txt", "working_directory": root,
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
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	startResult, err := runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"command":           backgroundLifecycleCommand(),
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

func TestBackgroundMutationRequiresExplicitMatchingCWD(t *testing.T) {
	runtime, workspaceID, root := newShellToolTestRuntime(t)
	result, err := runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id": workspaceID,
		"command":      "rm file.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "working_directory is required") {
		t.Fatalf("background mutation was not denied: %#v", result)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	result, err = runtime.Call(context.Background(), "start_process", map[string]any{
		"workspace_id":      workspaceID,
		"working_directory": root,
		"command":           "rm " + outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("outside background mutation was not denied: %#v", result)
	}
}
