package shell

import (
	"context"
	"encoding/hex"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProcessIDUsesCompactHex(t *testing.T) {
	id, err := processID()
	if err != nil {
		t.Fatal(err)
	}
	value := strings.TrimPrefix(id, "proc_")
	if len(value) != 16 {
		t.Fatalf("process id=%q", id)
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("process id is not hex: %q", id)
	}
}

func TestProcessManagerPrunesFinishedHistory(t *testing.T) {
	now := time.Now().UTC()
	code := 0
	manager := &ProcessManager{processes: map[string]*managedProcess{}, maxFinished: 2, retention: time.Hour}
	add := func(id string, finishedAt time.Time, running bool) {
		process := &managedProcess{id: id, finishedAt: finishedAt, stdout: &logBuffer{}, stderr: &logBuffer{}}
		if !running {
			value := code
			process.exitCode = &value
		}
		manager.processes[id] = process
		manager.order = append(manager.order, id)
	}
	add("expired", now.Add(-2*time.Hour), false)
	add("old", now.Add(-30*time.Minute), false)
	add("middle", now.Add(-20*time.Minute), false)
	add("recent", now.Add(-10*time.Minute), false)
	add("running", time.Time{}, true)

	manager.pruneLocked(now)
	for _, id := range []string{"expired", "old"} {
		if manager.processes[id] != nil {
			t.Fatalf("process %s was not pruned", id)
		}
	}
	for _, id := range []string{"middle", "recent", "running"} {
		if manager.processes[id] == nil {
			t.Fatalf("process %s was pruned unexpectedly", id)
		}
	}
	if len(manager.order) != 3 {
		t.Fatalf("order = %#v", manager.order)
	}
}

func TestProcessManagerRecordsBashProviderMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses system Bash path")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash unavailable")
	}
	manager, workspaceID, _ := newShellTestManager(t)
	resolver := NewProviderResolver(nil)
	resolver.goos = runtime.GOOS
	resolver.lookPath = func(name string) (string, error) { return bash, nil }
	manager.providers = resolver
	processes := NewProcessManagerWithExecutions(manager.workspaces, manager, manager.Executions())
	started, err := processes.Start(context.Background(), workspaceID, `printf "%s" "$SHELL"`)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err := processes.Status(workspaceID, started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(status) == 1 && !status[0].Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background process did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	output, err := processes.Output(workspaceID, started.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if output.Stdout != bash {
		t.Fatalf("SHELL = %q want %q", output.Stdout, bash)
	}
	snapshot, err := manager.Executions().Get(workspaceID, started.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Execution.Shell != "bash" || snapshot.Execution.ShellProvider != "system" {
		t.Fatalf("execution metadata = %#v", snapshot.Execution)
	}
}
