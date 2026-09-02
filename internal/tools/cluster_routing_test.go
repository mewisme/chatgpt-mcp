package tools

import (
	"context"
	"errors"
	"net/http/httptest"
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

func newTestClusterRuntime(t *testing.T, transport cluster.Transport, workspaceRoot string) testClusterRuntime {
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
	advertisement, err := runtime.ClusterAdvertisement()
	if err != nil {
		t.Fatal(err)
	}
	node := cluster.NewNode(transport, advertisement, runtime.ClusterRPCHandler)
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
	var receiverObservations, executorObservations []CallObservation
	first.runtime.CallObserver = func(value CallObservation) { receiverObservations = append(receiverObservations, value) }
	second.runtime.CallObserver = func(value CallObservation) { executorObservations = append(executorObservations, value) }
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
	firstID, _ := first.runtime.Workspaces.Instance()
	secondID, _ := second.runtime.Workspaces.Instance()
	assertClusterRouteObservation := func(label string, observations []CallObservation, source string) {
		t.Helper()
		for _, observation := range observations {
			if observation.Phase != "finish" || observation.Tool != "read_text_file" || observation.WorkspaceID != second.workspace.ID || observation.Source != source {
				continue
			}
			if observation.ReceivedByInstanceID != firstID.ID || observation.ExecutedByInstanceID != secondID.ID {
				t.Fatalf("%s route = received %q executed %q", label, observation.ReceivedByInstanceID, observation.ExecutedByInstanceID)
			}
			routing, _ := observation.Raw["routing"].(map[string]any)
			if routing["received_by_instance_id"] != firstID.ID || routing["executed_by_instance_id"] != secondID.ID {
				t.Fatalf("%s raw routing = %#v", label, routing)
			}
			return
		}
		t.Fatalf("%s remote execution observation missing: %#v", label, observations)
	}
	assertClusterRouteObservation("receiver", receiverObservations, "tunnel")
	assertClusterRouteObservation("executor", executorObservations, "cluster")
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
	advertisement, err := second.runtime.ClusterAdvertisement()
	if err != nil {
		t.Fatal(err)
	}
	advertisement.InstanceID = identity.ID
	advertisement.Name = identity.Name
	advertisement.Workspaces = []string{fakeWorkspaceID}
	second.node = cluster.NewNode(relay, advertisement, second.runtime.ClusterRPCHandler)
	second.runtime.SetClusterNode(second.node)
	second.node.SetAdvertisementProvider(nil)
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

func TestRuntimeRoutesWorkspaceToolCallOverWebSocketRelay(t *testing.T) {
	server := httptest.NewServer(cluster.NewRelayServer("cluster-secret"))
	defer server.Close()
	relayURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := newTestClusterRuntime(t, cluster.NewWebSocketTransport(relayURL, "cluster-secret"), t.TempDir())
	secondRoot := t.TempDir()
	second := newTestClusterRuntime(t, cluster.NewWebSocketTransport(relayURL, "cluster-secret"), secondRoot)
	startTestClusterRuntime(t, ctx, first)
	startTestClusterRuntime(t, ctx, second)
	if err := os.WriteFile(filepath.Join(secondRoot, "wire.txt"), []byte("over-websocket"), 0644); err != nil {
		t.Fatal(err)
	}
	var receiver, executor []CallObservation
	first.runtime.CallObserver = func(value CallObservation) { receiver = append(receiver, value) }
	second.runtime.CallObserver = func(value CallObservation) { executor = append(executor, value) }
	result, err := first.runtime.Call(WithCallSource(ctx, "tunnel"), "read_text_file", map[string]any{"workspace_id": second.workspace.ID, "path": "wire.txt"})
	if err != nil || result.IsError {
		t.Fatalf("WebSocket remote read failed: result=%#v err=%v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["content"] != "over-websocket" {
		t.Fatalf("structured content = %#v", result.StructuredContent)
	}
	firstIdentity, _ := first.runtime.Workspaces.Instance()
	secondIdentity, _ := second.runtime.Workspaces.Instance()
	assertRoute := func(label string, observations []CallObservation, source string) {
		t.Helper()
		for _, observation := range observations {
			if observation.Phase == "finish" && observation.Tool == "read_text_file" && observation.Source == source {
				if observation.ReceivedByInstanceID != firstIdentity.ID || observation.ExecutedByInstanceID != secondIdentity.ID {
					t.Fatalf("%s routing = %#v", label, observation)
				}
				return
			}
		}
		t.Fatalf("%s observation missing: %#v", label, observations)
	}
	assertRoute("receiver", receiver, "tunnel")
	assertRoute("executor", executor, "cluster")
}
