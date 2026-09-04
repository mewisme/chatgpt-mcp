package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestExecutionHubSnapshotsAndStreamsOutput(t *testing.T) {
	hub := NewExecutionHub()
	run := hub.Begin(ExecutionInput{WorkspaceID: "ws_test", Tool: "run_command", Command: "demo", CWD: "/tmp", Source: "mcp"})
	sub, snapshot, err := hub.Subscribe("ws_test", run.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Unsubscribe(sub)
	if snapshot.Execution.Status != ExecutionStatusRunning || snapshot.Execution.Source != "mcp" || snapshot.LatestSequence != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	_, _ = run.Writer("stdout").Write([]byte("hello\n"))
	event := <-sub.Events
	if event.Type != ExecutionEventOutput || event.Stream != "stdout" || event.Data != "hello\n" || event.Sequence != 1 {
		t.Fatalf("output event = %#v", event)
	}
	code := 0
	run.Finish(ExecutionStatusSuccess, &code, false)
	completed := <-sub.Events
	if completed.Type != ExecutionEventCompleted || completed.Status != ExecutionStatusSuccess || completed.Sequence != 2 {
		t.Fatalf("completed = %#v", completed)
	}
	final, err := hub.Get("ws_test", run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if final.Stdout != "hello\n" || final.Execution.Status != ExecutionStatusSuccess || final.Execution.ExitCode == nil || *final.Execution.ExitCode != 0 {
		t.Fatalf("final = %#v", final)
	}
	if _, err := hub.Get("ws_other", run.ID()); !errors.Is(err, ErrExecutionNotFound) {
		t.Fatalf("cross-workspace get err = %v", err)
	}
}

func TestRunCommandStreamsBeforeReturningAndPreservesFinalResult(t *testing.T) {
	if os.PathSeparator != '\\' && os.Getenv("SHELL") == "" {
		t.Setenv("SHELL", "/bin/sh")
	}
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	hub := NewExecutionHub()
	manager := NewManagerWithExecutions(workspaces, filepath.Join(t.TempDir(), "state"), hub)
	ctx := WithExecutionSource(context.Background(), "mcp")
	resultCh := make(chan ExecResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := manager.Exec(ctx, item.ID, streamingTestCommand())
		resultCh <- result
		errCh <- err
	}()

	var info ExecutionInfo
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		values := hub.List(item.ID, 10)
		if len(values) > 0 {
			info = values[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info.ID == "" {
		t.Fatal("execution was not registered while command was running")
	}
	sub, snapshot, err := hub.Subscribe(item.ID, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Unsubscribe(sub)
	streamed := snapshot.Stdout + snapshot.Stderr
	for !strings.Contains(streamed, "second") && time.Now().Before(deadline) {
		select {
		case event := <-sub.Events:
			streamed += event.Data
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !strings.Contains(streamed, "second") {
		t.Fatalf("streamed output = %q", streamed)
	}
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "first") || !strings.Contains(result.Stdout, "third") || !strings.Contains(result.Stderr, "second") || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	final, err := hub.Get(item.ID, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Execution.Status != ExecutionStatusSuccess || !strings.Contains(final.Stdout, "third") || !strings.Contains(final.Stderr, "second") {
		t.Fatalf("final = %#v", final)
	}
}

func streamingTestCommand() string {
	if os.PathSeparator == '\\' {
		return `Write-Output first; Start-Sleep -Milliseconds 150; [Console]::Error.WriteLine("second"); Start-Sleep -Milliseconds 150; Write-Output third`
	}
	return `printf 'first\n'; sleep 0.15; printf 'second\n' >&2; sleep 0.15; printf 'third\n'`
}
