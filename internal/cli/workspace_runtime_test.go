package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestWorkspaceRegisterSynchronizesRunningRuntime(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	runtimeManager := workspace.NewManager(workspace.DefaultStorePath())
	if items, err := runtimeManager.List(); err != nil || len(items) != 0 {
		t.Fatalf("initial items=%#v err=%v", items, err)
	}
	control, err := startRuntimeControl(runtimeControlOptions{
		Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil },
		ReloadWorkspaces: func() (workspaceReloadResult, error) {
			if err := runtimeManager.Reload(); err != nil {
				return workspaceReloadResult{}, err
			}
			items, err := runtimeManager.List()
			if err != nil {
				return workspaceReloadResult{}, err
			}
			return workspaceReloadResult{PID: os.Getpid(), Count: len(items)}, nil
		},
		Status:   func() runtimeStatusResult { return runtimeStatusResult{PID: os.Getpid()} },
		Shutdown: func() {}, ClearLogs: func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	workspaceRoot := t.TempDir()
	executeRequestCommand(t, root, []string{"workspace", "register", workspaceRoot})
	id := workspace.IDForPath(workspaceRoot)
	if _, err := runtimeManager.Get(id); err != nil {
		t.Fatalf("running runtime did not see registered workspace: %v", err)
	}
}
