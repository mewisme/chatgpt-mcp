package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func decodeStructured[T any](t *testing.T, value any) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRuntimeListShowsClusterMembers(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	result, err := first.runtime.Call(ctx, "runtime_list", map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("runtime_list failed: result=%#v err=%v", result, err)
	}
	value := result.StructuredContent.(RuntimeListResult)
	if value.Count != 2 || len(value.Runtimes) != 2 || !value.CatalogCompatible || value.CatalogHash == "" || value.CatalogError != "" {
		t.Fatalf("runtime list = %#v", value)
	}
	for _, member := range value.Runtimes {
		if !member.Online || member.InstanceID == "" || member.Name == "" {
			t.Fatalf("runtime member = %#v", member)
		}
	}
}

func TestWorkspaceListAggregatesOnlineInstances(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	result, err := first.runtime.Call(ctx, "workspace_list", map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("workspace_list failed: result=%#v err=%v", result, err)
	}
	value := result.StructuredContent.(WorkspaceListResult)
	if value.Count != 2 || len(value.Workspaces) != 2 {
		t.Fatalf("workspace list = %#v", value)
	}
	wanted := map[string]bool{first.workspace.ID: false, second.workspace.ID: false}
	for _, item := range value.Workspaces {
		if _, ok := wanted[item.WorkspaceID]; ok {
			wanted[item.WorkspaceID] = item.Online && item.InstanceID != "" && item.InstanceName != "" && item.WorkspaceRoot != ""
		}
	}
	for id, found := range wanted {
		if !found {
			t.Fatalf("workspace %s missing or incomplete: %#v", id, value)
		}
	}
}

func TestWorkspaceRegisterTargetsRemoteInstanceAndRefreshesOwnership(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	secondIdentity, err := second.runtime.Workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	remoteRoot := t.TempDir()
	result, err := first.runtime.Call(ctx, "workspace_register", map[string]any{"instance_id": secondIdentity.ID, "path": remoteRoot})
	if err != nil || result.IsError {
		t.Fatalf("remote workspace register failed: result=%#v err=%v", result, err)
	}
	registered := decodeStructured[WorkspaceRegistrationResult](t, result.StructuredContent)
	if registered.InstanceID != secondIdentity.ID || filepath.Clean(registered.WorkspaceRoot) != filepath.Clean(remoteRoot) || registered.WorkspaceID == "" {
		t.Fatalf("registration = %#v", registered)
	}
	if _, err := first.runtime.Workspaces.Get(registered.WorkspaceID); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("remote workspace unexpectedly exists on receiving instance: %v", err)
	}
	if item, err := second.runtime.Workspaces.Get(registered.WorkspaceID); err != nil || filepath.Clean(item.Path) != filepath.Clean(remoteRoot) {
		t.Fatalf("remote owner workspace = %#v err=%v", item, err)
	}
	owner, err := first.node.WorkspaceOwner(ctx, registered.WorkspaceID)
	if err != nil || owner.InstanceID != secondIdentity.ID || !owner.Online {
		t.Fatalf("owner = %#v err=%v", owner, err)
	}
	write, err := first.runtime.Call(ctx, "write_file", map[string]any{"workspace_id": registered.WorkspaceID, "path": "routed.txt", "content": "remote"})
	if err != nil || write.IsError {
		t.Fatalf("routed write failed: result=%#v err=%v", write, err)
	}
	data, err := os.ReadFile(filepath.Join(remoteRoot, "routed.txt"))
	if err != nil || string(data) != "remote" {
		t.Fatalf("remote routed file = %q err=%v", data, err)
	}
}

func TestWorkspaceStatusReportsRemoteInstanceMetadata(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	identity, err := second.runtime.Workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	result, err := first.runtime.Call(ctx, "workspace_status", map[string]any{"workspace_id": second.workspace.ID})
	if err != nil || result.IsError {
		t.Fatalf("remote workspace status failed: result=%#v err=%v", result, err)
	}
	status := decodeStructured[WorkspaceStatusResult](t, result.StructuredContent)
	if status.InstanceID != identity.ID || status.InstanceName != identity.Name || !status.Online || status.WorkspaceID != second.workspace.ID {
		t.Fatalf("workspace status = %#v", status)
	}
}

func TestWorkspaceRegisterRejectsOfflineTarget(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	if err := second.node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	identity, err := second.runtime.Workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	if err := second.node.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := first.runtime.Call(ctx, "workspace_register", map[string]any{"instance_id": identity.ID, "path": t.TempDir()})
	if err != nil {
		t.Fatalf("offline target returned protocol error: %v", err)
	}
	if !result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "cluster instance is offline") {
		t.Fatalf("offline target result = %#v", result)
	}
}

func TestStandaloneRuntimeAndWorkspaceList(t *testing.T) {
	runtime, workspaceID, root := newToolTestRuntime(t)
	RegisterClusterTools(runtime.Registry, runtime)
	runtimes, err := runtime.Call(context.Background(), "runtime_list", map[string]any{})
	if err != nil || runtimes.IsError {
		t.Fatalf("runtime_list failed: %#v err=%v", runtimes, err)
	}
	runtimeList := runtimes.StructuredContent.(RuntimeListResult)
	if runtimeList.Count != 1 || !runtimeList.Runtimes[0].Online || !runtimeList.CatalogCompatible || runtimeList.CatalogHash == "" {
		t.Fatalf("runtime list = %#v", runtimeList)
	}
	workspaces, err := runtime.Call(context.Background(), "workspace_list", map[string]any{})
	if err != nil || workspaces.IsError {
		t.Fatalf("workspace_list failed: %#v err=%v", workspaces, err)
	}
	workspaceList := workspaces.StructuredContent.(WorkspaceListResult)
	if workspaceList.Count != 1 || workspaceList.Workspaces[0].WorkspaceID != workspaceID || filepath.Clean(workspaceList.Workspaces[0].WorkspaceRoot) != filepath.Clean(root) {
		t.Fatalf("workspace list = %#v", workspaceList)
	}
}
