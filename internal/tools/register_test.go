package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
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
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "state"))
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, manager)
	RegisterCore(registry, manager, checkpoints)
	return &Runtime{Registry: registry, Workspaces: manager, Checkpoints: checkpoints}, item.ID, root
}

func callTool(t *testing.T, runtime *Runtime, name string, args map[string]any) Result {
	t.Helper()
	result, err := runtime.Call(context.Background(), name, args)
	if err != nil {
		t.Fatalf("%s protocol error: %v", name, err)
	}
	return result
}

func baseArgs(workspaceID, root string) map[string]any {
	return map[string]any{"workspace_id": workspaceID, "working_directory": root}
}

func TestReadTextFilePartialLines(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("one\ntwo\nthree\nfour"), 0644); err != nil {
		t.Fatal(err)
	}
	args := baseArgs(workspaceID, root)
	args["path"] = "file.txt"
	args["offset"] = 2
	args["limit"] = 2
	result := callTool(t, runtime, "read_text_file", args)
	value := result.StructuredContent.(ReadTextFileResult)
	if value.Content != "     2|two\n     3|three" {
		t.Fatalf("content = %q", value.Content)
	}
	if value.Lines == nil || *value.Lines != 2 {
		t.Fatalf("lines = %#v", value.Lines)
	}
}

func TestReadFileBase64Chunk(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	data := []byte("abcdefghij")
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	args := baseArgs(workspaceID, root)
	args["path"] = "binary.bin"
	args["offset"] = 2
	args["length"] = 4
	result := callTool(t, runtime, "read_file_base64", args)
	value := result.StructuredContent.(ReadFileBase64Result)
	if decoded, _ := base64.StdEncoding.DecodeString(value.Content); string(decoded) != "cdef" {
		t.Fatalf("chunk = %q", decoded)
	}
	if value.NextOffset == nil || *value.NextOffset != 6 || value.Done {
		t.Fatalf("unexpected offsets: %#v", value)
	}
}

func TestWriteAndEditCreateCheckpoints(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	args := baseArgs(workspaceID, root)
	args["path"] = "file.txt"
	args["content"] = "alpha beta"
	result := callTool(t, runtime, "write_file", args)
	write := result.StructuredContent.(WriteFileResult)
	if write.CheckpointID == nil {
		t.Fatal("write_file did not create checkpoint")
	}

	editArgs := baseArgs(workspaceID, root)
	editArgs["path"] = "file.txt"
	editArgs["old_text"] = "beta"
	editArgs["new_text"] = "gamma"
	result = callTool(t, runtime, "edit_file", editArgs)
	edit := result.StructuredContent.(EditFileResult)
	if edit.CheckpointID == nil || !strings.Contains(edit.Diff, "+ alpha gamma") {
		t.Fatalf("unexpected edit result: %#v", edit)
	}
	data, err := os.ReadFile(filepath.Join(root, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha gamma" {
		t.Fatalf("content = %q", data)
	}
}

func TestEditDryRunDoesNotWriteOrCheckpoint(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	args := baseArgs(workspaceID, root)
	args["path"] = "file.txt"
	args["old_text"] = "old"
	args["new_text"] = "new"
	args["dry_run"] = true
	result := callTool(t, runtime, "edit_file", args)
	value := result.StructuredContent.(EditFileResult)
	if value.CheckpointID != nil || !value.DryRun {
		t.Fatalf("unexpected dry run: %#v", value)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "old" {
		t.Fatalf("dry-run mutated file: %q", data)
	}
}

func TestMultiFilePatchRejectsEscapeBeforeMutation(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	file := filepath.Join(root, "safe.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	args := baseArgs(workspaceID, root)
	args["patch"] = "*** Begin Patch\n*** Update File: safe.txt\n@@\n-old\n+new\n*** Add File: ../escape.txt\n+bad\n*** End Patch"
	result := callTool(t, runtime, "apply_patch", args)
	if !result.IsError {
		t.Fatalf("escape patch was not rejected: %#v", result)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "old\n" {
		t.Fatalf("safe file mutated before validation: %q", data)
	}
}

func TestGlobAndGrep(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.ts"), []byte("const hello = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "b.go"), []byte("package test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	globArgs := baseArgs(workspaceID, root)
	globArgs["pattern"] = "**/*.ts"
	globResult := callTool(t, runtime, "glob", globArgs).StructuredContent.(GlobResult)
	if len(globResult.Matches) != 1 || !strings.HasSuffix(globResult.Matches[0], filepath.Join("src", "a.ts")) {
		t.Fatalf("glob = %#v", globResult)
	}

	grepArgs := baseArgs(workspaceID, root)
	grepArgs["pattern"] = "hello"
	grepArgs["glob"] = "*.ts"
	grepResult := callTool(t, runtime, "grep", grepArgs).StructuredContent.(GrepResult)
	if !strings.Contains(grepResult.Output, "a.ts:1") {
		t.Fatalf("grep = %#v", grepResult)
	}
}

func TestDeleteAndMoveStayInsideWorkspace(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	moveArgs := baseArgs(workspaceID, root)
	moveArgs["source"] = "a.txt"
	moveArgs["destination"] = filepath.Join(t.TempDir(), "outside.txt")
	result := callTool(t, runtime, "move_file", moveArgs)
	if !result.IsError {
		t.Fatalf("outside move was not rejected: %#v", result)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("source changed after denied move: %v", err)
	}

	deleteArgs := baseArgs(workspaceID, root)
	deleteArgs["path"] = filepath.Join(t.TempDir(), "outside.txt")
	result = callTool(t, runtime, "delete_file", deleteArgs)
	if !result.IsError {
		t.Fatalf("outside delete was not rejected: %#v", result)
	}
}

func TestRunCommandMutationGuardStillApplies(t *testing.T) {
	if os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	runtime, workspaceID, root := newToolTestRuntime(t)
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	args := baseArgs(workspaceID, root)
	args["command"] = "cd child && rm file.txt"
	result := callTool(t, runtime, "run_command", args)
	if !result.IsError || !strings.Contains(result.Content[0].Text, "cwd change") {
		t.Fatalf("cwd-changing mutation was not denied: %#v", result)
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
	args := baseArgs(workspaceID, root)
	result := callTool(t, runtime, "git_status", args)
	value := result.StructuredContent.(GitStatusResult)
	if !strings.Contains(value.Output, "untracked.txt") {
		t.Fatalf("git status = %#v", value)
	}
}

func TestFilesystemToolCatalog(t *testing.T) {
	runtime, _, _ := newToolTestRuntime(t)
	names := map[string]bool{}
	for _, schema := range runtime.List() {
		names[schema.Name] = true
	}
	for _, name := range []string{
		"read_text_file", "read_file_base64", "write_file", "write_file_base64", "edit_file", "multi_edit",
		"replace_regex", "apply_patch", "list_directory", "glob", "grep", "delete_file", "create_directory",
		"delete_directory", "copy_file", "move_file", "search_files", "directory_tree", "list_allowed_directories",
		"read_files",
	} {
		if !names[name] {
			t.Fatalf("missing tool %q", name)
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

func TestAllowedDirectorySupportsFilesystemAndRewind(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddAllowDir(item.ID, allowed); err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "state"))
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, manager)
	RegisterCore(registry, manager, checkpoints)
	runtime := &Runtime{Registry: registry, Workspaces: manager, Checkpoints: checkpoints}

	write := callTool(t, runtime, "write_file", map[string]any{"workspace_id": item.ID, "working_directory": allowed, "path": "artifact.txt", "content": "before"})
	if write.IsError || write.StructuredContent.(WriteFileResult).CheckpointID == nil {
		t.Fatalf("allowed write failed: %#v", write)
	}
	edit := callTool(t, runtime, "edit_file", map[string]any{"workspace_id": item.ID, "working_directory": allowed, "path": "artifact.txt", "old_text": "before", "new_text": "after"})
	editResult := edit.StructuredContent.(EditFileResult)
	if edit.IsError || editResult.CheckpointID == nil {
		t.Fatalf("allowed edit failed: %#v", edit)
	}
	restore := callTool(t, runtime, "rewind", map[string]any{"workspace_id": item.ID, "action": "restore", "checkpoint_id": *editResult.CheckpointID})
	if restore.IsError {
		t.Fatalf("allowed rewind failed: %#v", restore)
	}
	data, err := os.ReadFile(filepath.Join(allowed, "artifact.txt"))
	if err != nil || string(data) != "before" {
		t.Fatalf("restored content = %q err=%v", data, err)
	}

	edit = callTool(t, runtime, "edit_file", map[string]any{"workspace_id": item.ID, "working_directory": allowed, "path": "artifact.txt", "old_text": "before", "new_text": "after"})
	editResult = edit.StructuredContent.(EditFileResult)
	if _, err := manager.RemoveAllowDir(item.ID, allowed); err != nil {
		t.Fatal(err)
	}
	preview := callTool(t, runtime, "rewind", map[string]any{"workspace_id": item.ID, "action": "preview", "checkpoint_id": *editResult.CheckpointID})
	if !preview.IsError {
		t.Fatalf("revoked allow dir remained rewind-accessible: %#v", preview)
	}
}
