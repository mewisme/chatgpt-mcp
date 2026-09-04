package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newSessionBindingRuntime(t *testing.T) (*Runtime, string, string, *int) {
	t.Helper()
	manager := workspace.NewManager(t.TempDir() + "/workspaces.json")
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	registry := NewRegistry()
	registry.MustRegister("workspace_probe", Schema{Name: "workspace_probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"}},"required":["workspace_id"]}`)}, func(_ context.Context, args map[string]any) (Result, error) {
		calls++
		workspaceID, _ := args["workspace_id"].(string)
		return TextResult(workspaceID), nil
	})
	registry.MustRegister("global_probe", Schema{Name: "global_probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (Result, error) {
		return TextResult("ok"), nil
	})
	return &Runtime{Registry: registry, Workspaces: manager, SessionBindings: NewSessionWorkspaceBinder()}, first.ID, second.ID, &calls
}

func TestRuntimeBindsSessionToFirstValidWorkspace(t *testing.T) {
	runtime, first, second, calls := newSessionBindingRuntime(t)
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": first})
	if err != nil || result.IsError || *calls != 1 {
		t.Fatalf("first call = %#v err=%v calls=%d", result, err, *calls)
	}
	result, err = runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": first})
	if err != nil || result.IsError || *calls != 2 {
		t.Fatalf("repeat call = %#v err=%v calls=%d", result, err, *calls)
	}
	result, err = runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": second})
	if err != nil || !result.IsError || *calls != 2 || !strings.Contains(result.Content[0].Text, "cannot access") {
		t.Fatalf("switch call = %#v err=%v calls=%d", result, err, *calls)
	}
	binding, ok := runtime.SessionBindings.Lookup("session-a")
	if !ok || binding.WorkspaceID != first {
		t.Fatalf("binding = %#v ok=%t", binding, ok)
	}
}

func TestRuntimeAllowsManySessionsOnSameWorkspace(t *testing.T) {
	runtime, first, _, calls := newSessionBindingRuntime(t)
	for _, sessionID := range []string{"session-a", "session-b"} {
		result, err := runtime.Call(WithMCPSessionID(context.Background(), sessionID), "workspace_probe", map[string]any{"workspace_id": first})
		if err != nil || result.IsError {
			t.Fatalf("session %s = %#v err=%v", sessionID, result, err)
		}
	}
	if *calls != 2 {
		t.Fatalf("calls = %d", *calls)
	}
}

func TestRuntimeInvalidWorkspaceDoesNotBindSession(t *testing.T) {
	runtime, _, second, calls := newSessionBindingRuntime(t)
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": "ws_missing"})
	if err != nil || !result.IsError || *calls != 0 {
		t.Fatalf("invalid workspace = %#v err=%v calls=%d", result, err, *calls)
	}
	if _, ok := runtime.SessionBindings.Lookup("session-a"); ok {
		t.Fatal("invalid workspace created a session binding")
	}
	result, err = runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": second})
	if err != nil || result.IsError || *calls != 1 {
		t.Fatalf("valid workspace after invalid = %#v err=%v calls=%d", result, err, *calls)
	}
}

func TestRuntimeWorkspaceSchemaRequiresExplicitWorkspaceID(t *testing.T) {
	runtime, _, _, calls := newSessionBindingRuntime(t)
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "workspace_probe", map[string]any{})
	if err != nil || !result.IsError || *calls != 0 || !strings.Contains(result.Content[0].Text, "workspace_id") {
		t.Fatalf("missing workspace = %#v err=%v calls=%d", result, err, *calls)
	}
	if _, ok := runtime.SessionBindings.Lookup("session-a"); ok {
		t.Fatal("missing workspace created a session binding")
	}
}

func TestRuntimeIgnoresWorkspaceIDOnGlobalTool(t *testing.T) {
	runtime, first, _, _ := newSessionBindingRuntime(t)
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "global_probe", map[string]any{"workspace_id": first})
	if err != nil || result.IsError {
		t.Fatalf("global tool = %#v err=%v", result, err)
	}
	if _, ok := runtime.SessionBindings.Lookup("session-a"); ok {
		t.Fatal("global tool created a session binding from undeclared workspace_id")
	}
}

func TestRuntimeNonWorkspaceToolDoesNotBindSession(t *testing.T) {
	runtime, _, _, _ := newSessionBindingRuntime(t)
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "global_probe", map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("global tool = %#v err=%v", result, err)
	}
	if _, ok := runtime.SessionBindings.Lookup("session-a"); ok {
		t.Fatal("global tool created a session binding")
	}
}

func TestRuntimeObservesSessionBindingWithoutRawSessionID(t *testing.T) {
	runtime, first, second, _ := newSessionBindingRuntime(t)
	observed := make(chan CallObservation, 4)
	runtime.SetCallObserver(func(value CallObservation) { observed <- value })
	ctx := WithMCPSessionID(context.Background(), "session-secret-a")
	if result, err := runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": first}); err != nil || result.IsError {
		t.Fatalf("first call = %#v err=%v", result, err)
	}
	start, finish := <-observed, <-observed
	if !strings.HasPrefix(start.CallID, "call_") || strings.Count(start.CallID, "_") != 2 || finish.CallID != start.CallID {
		t.Fatalf("call ids = %q / %q", start.CallID, finish.CallID)
	}
	if start.SessionBinding != SessionBindingNew || finish.SessionBinding != SessionBindingNew || finish.SessionWorkspaceID != first {
		t.Fatalf("new binding observations = %#v / %#v", start, finish)
	}
	if finish.SessionHash == "" || finish.SessionHash != MCPSessionFingerprint("session-secret-a") {
		t.Fatalf("session hash = %q", finish.SessionHash)
	}
	if result, err := runtime.Call(ctx, "workspace_probe", map[string]any{"workspace_id": second}); err != nil || !result.IsError {
		t.Fatalf("denied call = %#v err=%v", result, err)
	}
	_, denied := <-observed, <-observed
	if denied.SessionBinding != SessionBindingDenied || denied.SessionWorkspaceID != first || denied.WorkspaceID != second || denied.Status != "error" {
		t.Fatalf("denied observation = %#v", denied)
	}
	data, err := json.Marshal(denied.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "session-secret-a") {
		t.Fatalf("raw observation leaked MCP session id: %s", data)
	}
}

func TestRuntimeDoesNotBindExternalUpstreamWorkspaceID(t *testing.T) {
	runtime, _, _, _ := newSessionBindingRuntime(t)
	if err := runtime.Registry.ReplaceOwned("upstream:test", map[string]Entry{
		"external_probe": {
			Schema:  Schema{Name: "external_probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"}},"required":["workspace_id"]}`)},
			Handler: func(context.Context, map[string]any) (Result, error) { return TextResult("ok"), nil },
		},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := WithMCPSessionID(context.Background(), "session-a")
	result, err := runtime.Call(ctx, "external_probe", map[string]any{"workspace_id": "external-workspace"})
	if err != nil || result.IsError {
		t.Fatalf("external tool = %#v err=%v", result, err)
	}
	if _, ok := runtime.SessionBindings.Lookup("session-a"); ok {
		t.Fatal("external upstream workspace_id created a local session binding")
	}
}
