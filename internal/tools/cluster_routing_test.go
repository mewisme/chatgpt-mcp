package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/cluster"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type testClusterRuntime struct {
	runtime   *Runtime
	workspace workspace.Workspace
	node      *cluster.Node
}

func newTestClusterRuntime(t *testing.T, relay *cluster.MemoryRelay, workspaceRoot string) testClusterRuntime {
	t.Helper()
	configRoot := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(configRoot, "workspaces.json"))
	item, err := workspaces.Register(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(configRoot)
	registry := NewRegistry()
	shell := shellruntime.NewManager(workspaces, configRoot)
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterCore(registry, workspaces, checkpoints, shell)
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}
	RegisterClusterTools(registry, runtime)
	identity, err := workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	node := cluster.NewNode(relay, cluster.Advertisement{InstanceID: identity.ID, Name: identity.Name, Workspaces: []string{item.ID}}, runtime.ClusterRPCHandler)
	runtime.SetClusterNode(node)
	return testClusterRuntime{runtime: runtime, workspace: item, node: node}
}

func startTestClusterRuntime(t *testing.T, ctx context.Context, value testClusterRuntime) {
	t.Helper()
	if err := value.node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.node.Close() })
}

func TestRuntimeRoutesWorkspaceToolCallToRemoteOwner(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rootA, rootB := t.TempDir(), t.TempDir()
	first := newTestClusterRuntime(t, relay, rootA)
	second := newTestClusterRuntime(t, relay, rootB)
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	if err := os.WriteFile(filepath.Join(rootB, "owner.txt"), []byte("from-b"), 0644); err != nil {
		t.Fatal(err)
	}
	var observations []CallObservation
	second.runtime.CallObserver = func(value CallObservation) { observations = append(observations, value) }
	result, err := first.runtime.Call(WithCallSource(ctx, "tunnel"), "read_text_file", map[string]any{"workspace_id": second.workspace.ID, "path": "owner.txt"})
	if err != nil || result.IsError {
		t.Fatalf("remote read failed: result=%#v err=%v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["content"] != "from-b" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
	if _, err := first.runtime.Workspaces.Get(second.workspace.ID); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("remote workspace unexpectedly registered locally: %v", err)
	}
	foundClusterExecution := false
	for _, observation := range observations {
		if observation.Phase == "finish" && observation.Tool == "read_text_file" && observation.WorkspaceID == second.workspace.ID && observation.Source == "cluster" {
			foundClusterExecution = true
		}
	}
	if !foundClusterExecution {
		t.Fatalf("remote execution observation missing: %#v", observations)
	}
}

func TestRuntimeRoutesMutationOnlyToRemoteOwner(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rootA, rootB := t.TempDir(), t.TempDir()
	first := newTestClusterRuntime(t, relay, rootA)
	second := newTestClusterRuntime(t, relay, rootB)
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	result, err := first.runtime.Call(ctx, "write_file", map[string]any{"workspace_id": second.workspace.ID, "path": "remote.txt", "content": "written-on-b"})
	if err != nil || result.IsError {
		t.Fatalf("remote write failed: result=%#v err=%v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(rootB, "remote.txt"))
	if err != nil || string(data) != "written-on-b" {
		t.Fatalf("remote file = %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "remote.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mutation leaked to receiving instance: %v", err)
	}
}

func TestRuntimeFailsClosedWhenWorkspaceOwnerIsOffline(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	if err := second.node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.node.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := first.runtime.Call(ctx, "read_text_file", map[string]any{"workspace_id": second.workspace.ID, "path": "missing.txt"})
	if err != nil {
		t.Fatalf("owner failure returned protocol error: %v", err)
	}
	if !result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "workspace owner is offline") {
		t.Fatalf("offline owner result = %#v", result)
	}
}

func TestClusterRPCRejectsWorkspaceNotRegisteredOnTarget(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	identity, err := second.runtime.Workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	fakeWorkspaceID := "ws_remote_missing"
	second.node = cluster.NewNode(relay, cluster.Advertisement{InstanceID: identity.ID, Name: identity.Name, Workspaces: []string{fakeWorkspaceID}}, second.runtime.ClusterRPCHandler)
	second.runtime.SetClusterNode(second.node)
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	result, err := first.runtime.Call(ctx, "read_text_file", map[string]any{"workspace_id": fakeWorkspaceID, "path": "anything.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "not owned by this instance") {
		t.Fatalf("misrouted result = %#v", result)
	}
}

func TestRemoteToolNotFoundPreservesProtocolError(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, relay, t.TempDir())
	second := newTestClusterRuntime(t, relay, t.TempDir())
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	_, err := first.runtime.Call(ctx, "missing_tool", map[string]any{"workspace_id": second.workspace.ID})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("error = %v, want ErrToolNotFound", err)
	}
}
