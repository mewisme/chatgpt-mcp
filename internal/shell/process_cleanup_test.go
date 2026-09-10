package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestClearFinishedProcessRejectsRunningAndDeletesFinished(t *testing.T) {
	if os.PathSeparator != '\\' && os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shell := NewManager(manager, filepath.Join(t.TempDir(), "shell-state"))
	processes := NewProcessManager(manager, shell)
	command := "sleep 0.2"
	if os.PathSeparator == '\\' {
		command = "Start-Sleep -Milliseconds 200"
	}
	started, err := processes.Start(t.Context(), item.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := processes.ClearFinished(item.ID, started.ID); !errors.Is(err, ErrProcessRunning) {
		t.Fatalf("running cleanup error=%v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, statusErr := processes.Status(item.ID, started.ID)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if len(status) == 1 && !status[0].Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := processes.ClearFinished(item.ID, started.ID); err != nil {
		t.Fatal(err)
	}
	status, err := processes.Status(item.ID, started.ID)
	if err != nil || len(status) != 0 {
		t.Fatalf("process still present=%#v err=%v", status, err)
	}
}
