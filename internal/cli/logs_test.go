package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestLogsTailAppliesAfterVisibilityFiltering(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)
	for _, event := range []runtimeevent.Event{
		{Sequence: 1, Time: base, RunID: "run", Level: "info", Kind: "success", Name: "server.first", Component: "SERVER", Message: "First visible"},
		{Sequence: 2, Time: base.Add(time.Second), RunID: "run", Level: "info", Visibility: logger.VisibilityVerbose, Kind: "info", Name: "server.verbose", Component: "SERVER", Message: "Verbose hidden"},
		{Sequence: 3, Time: base.Add(2 * time.Second), RunID: "run", Level: "info", Kind: "success", Name: "server.last", Component: "SERVER", Message: "Last visible"},
	} {
		if err := journal.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	output := executeLogsCommand(t, root, []string{"logs", "-n", "2"})
	if !strings.Contains(output, "First visible") || !strings.Contains(output, "Last visible") || strings.Contains(output, "Verbose hidden") {
		t.Fatalf("default logs output = %q", output)
	}
	debugOutput := executeLogsCommand(t, root, []string{"--debug", "logs", "-n", "3"})
	if !strings.Contains(debugOutput, "Verbose hidden") {
		t.Fatalf("debug logs did not include hidden event: %q", debugOutput)
	}
}

func TestLogsWorkspacePathFilter(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := t.TempDir()
	manager := workspace.NewManager(workspace.DefaultStorePath())
	item, err := manager.Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []runtimeevent.Event{
		{Time: time.Now(), Level: "info", Kind: "success", Name: "tool.call.completed", Component: "TOOL", Message: "Wanted", WorkspaceID: item.ID, Tool: "run_command"},
		{Time: time.Now(), Level: "info", Kind: "success", Name: "tool.call.completed", Component: "TOOL", Message: "Other", WorkspaceID: "ws_other", Tool: "run_command"},
	} {
		if err := journal.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	output := executeLogsCommand(t, root, []string{"logs", "--workspace", workspaceRoot})
	if !strings.Contains(output, "Wanted") || strings.Contains(output, "Other") {
		t.Fatalf("workspace logs output = %q", output)
	}
}

func TestLogsFollowStreamsRuntimeEvents(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	stream := runtimeevent.NewStream(runtimeevent.Metadata{RunID: "run_follow", PID: os.Getpid()})
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_follow", Events: stream, Reload: func(context.Context) (runtimeReloadResult, error) { return runtimeReloadResult{PID: os.Getpid()}, nil }, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_follow", ConfigRoot: root}
	}, Shutdown: func() {}, ClearLogs: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "logs", "-f", "-n", "0"})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	time.Sleep(100 * time.Millisecond)
	if err := stream.WriteEvent(logger.Event{Time: time.Now(), Level: logger.Info, Kind: logger.KindSuccess, Name: "server.follow", Component: "SERVER", Message: "Followed live event"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("logs follow did not stop after context cancellation")
	}
	if !strings.Contains(output.String(), "Followed live event") {
		t.Fatalf("follow output = %q", output.String())
	}
}

func TestLogsPathAndClearWhileStopped(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(runtimeevent.Event{Time: time.Now(), Level: "info", Kind: "info", Name: "test.event", Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	pathOutput := executeLogsCommand(t, root, []string{"logs", "path"})
	if !strings.Contains(pathOutput, runtimeevent.Path(root)) {
		t.Fatalf("logs path output = %q", pathOutput)
	}
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "logs", "clear"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("logs clear without force = %v", err)
	}
	cmd = newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config-dir", root, "logs", "clear", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runtimeevent.Path(root)); !os.IsNotExist(err) {
		t.Fatalf("runtime journal still exists after clear: %v", err)
	}
}

func TestLogsClearUsesRuntimeControlWhenRunning(t *testing.T) {
	defer configformat.SetRootPath("")
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	clearCalls := 0
	control, err := startRuntimeControl(runtimeControlOptions{RunID: "run_clear", Events: runtimeevent.NewStream(runtimeevent.Metadata{}), Reload: func(context.Context) (runtimeReloadResult, error) {
		return runtimeReloadResult{PID: os.Getpid()}, nil
	}, Status: func() runtimeStatusResult {
		return runtimeStatusResult{PID: os.Getpid(), RunID: "run_clear", ConfigRoot: root}
	}, Shutdown: func() {}, ClearLogs: func() error {
		clearCalls++
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	output := executeLogsCommand(t, root, []string{"logs", "clear", "--force"})
	if clearCalls != 1 || !strings.Contains(output, "Runtime logs cleared") {
		t.Fatalf("clear calls=%d output=%q", clearCalls, output)
	}
}

func executeLogsCommand(t *testing.T, root string, args []string) string {
	t.Helper()
	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(append([]string{"--config-dir", root}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

var _ = filepath.Separator
