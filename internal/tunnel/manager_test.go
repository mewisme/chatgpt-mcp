package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestManagerSharesRuntimeAndKeepsUnchangedClients(t *testing.T) {
	runtime := &tools.Runtime{}
	manager := NewManager(runtime, nil)
	initial := CollectionConfig{Instances: []InstanceConfig{
		{ID: "tunnel_a", APIKey: "key-a"},
		{ID: "tunnel_b", APIKey: "key-b"},
	}}
	if err := manager.Reconcile(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	a, _ := manager.Client("tunnel_a")
	b, _ := manager.Client("tunnel_b")
	if a == b || a.runtime != runtime || b.runtime != runtime {
		t.Fatal("clients must be distinct and share the runtime")
	}
	next := CollectionConfig{Instances: []InstanceConfig{
		{ID: "tunnel_a", APIKey: "key-a"},
		{ID: "tunnel_b", APIKey: "new-key"},
	}}
	if err := manager.Reconcile(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	afterA, _ := manager.Client("tunnel_a")
	afterB, _ := manager.Client("tunnel_b")
	if afterA != a || afterB == b {
		t.Fatal("reconcile must preserve unchanged client and replace changed client")
	}
}

func TestManagerReconcileAddsAndRemovesOnlyChangedInstances(t *testing.T) {
	runtime := &tools.Runtime{}
	manager := NewManager(runtime, nil)
	if err := manager.Reconcile(context.Background(), CollectionConfig{Instances: []InstanceConfig{{ID: "a", APIKey: "key-a"}, {ID: "b", APIKey: "key-b"}}}); err != nil {
		t.Fatal(err)
	}
	a, _ := manager.Client("a")
	b, _ := manager.Client("b")
	if err := manager.Reconcile(context.Background(), CollectionConfig{Instances: []InstanceConfig{{ID: "a", APIKey: "key-a"}, {ID: "b", APIKey: "key-b"}, {ID: "c", APIKey: "key-c"}}}); err != nil {
		t.Fatal(err)
	}
	afterAddA, _ := manager.Client("a")
	afterAddB, _ := manager.Client("b")
	c, ok := manager.Client("c")
	if !ok || afterAddA != a || afterAddB != b {
		t.Fatal("adding C replaced an unchanged client")
	}
	if err := manager.Reconcile(context.Background(), CollectionConfig{Instances: []InstanceConfig{{ID: "a", APIKey: "key-a"}, {ID: "c", APIKey: "key-c"}}}); err != nil {
		t.Fatal(err)
	}
	afterRemoveA, _ := manager.Client("a")
	afterRemoveC, _ := manager.Client("c")
	if afterRemoveA != a || afterRemoveC != c {
		t.Fatal("removing B replaced an unchanged client")
	}
	if _, ok := manager.Client("b"); ok {
		t.Fatal("removed tunnel B is still attached")
	}
}

func TestManagerStartsHealthyTunnelWhenAnotherFails(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	m := NewManager(runtime, nil)
	m.factory = func(cfg Config, _ sdkmcp.Transport) (Backend, error) {
		fake := newFakeBackend()
		if cfg.ID == "bad" {
			fake.startErr = errors.New("backend unavailable")
		}
		return fake, nil
	}
	cfg := CollectionConfig{Instances: []InstanceConfig{{ID: "bad", APIKey: "key", Enabled: true}, {ID: "good", APIKey: "key", Enabled: true}}}
	if err := m.Reconcile(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.StartContext(context.Background()); err == nil {
		t.Fatal("expected failed tunnel error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.WaitUntilAnyReady(ctx); err != nil {
		t.Fatal(err)
	}
	good, _ := m.Client("good")
	if !good.Status().Ready {
		t.Fatal("healthy tunnel was affected by failed tunnel")
	}
	if err := m.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestManagerReconcileRollsBackChangedTunnelOnly(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	m := NewManager(runtime, nil)
	m.factory = func(cfg Config, _ sdkmcp.Transport) (Backend, error) {
		fake := newFakeBackend()
		if cfg.APIKey == "bad-key" {
			fake.startErr = errors.New("backend unavailable")
		}
		return fake, nil
	}
	initial := CollectionConfig{Instances: []InstanceConfig{{ID: "a", APIKey: "old-key", Enabled: true}, {ID: "b", APIKey: "old-key", Enabled: true}}}
	if err := m.Reconcile(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	if err := m.StartContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), time.Second)
	defer readyCancel()
	if err := m.WaitUntilAnyReady(readyCtx); err != nil {
		t.Fatal(err)
	}
	a, _ := m.Client("a")
	b, _ := m.Client("b")
	changed := CollectionConfig{Instances: []InstanceConfig{{ID: "a", APIKey: "bad-key", Enabled: true}, {ID: "b", APIKey: "old-key", Enabled: true}}}
	if err := m.Reconcile(context.Background(), changed); err == nil {
		t.Fatal("expected reconcile error")
	}
	afterA, _ := m.Client("a")
	afterB, _ := m.Client("b")
	if err := a.WaitUntilReady(readyCtx); err != nil {
		t.Fatal(err)
	}
	if err := b.WaitUntilReady(readyCtx); err != nil {
		t.Fatal(err)
	}
	if afterA != a || afterB != b || !b.Status().Ready || !a.Status().Ready {
		t.Fatalf("failed reconcile changed healthy manager state: a=%+v b=%+v", a.Status(), b.Status())
	}
	if err := m.StopContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteSessionIDsAreIsolatedByTunnel(t *testing.T) {
	registry := tools.NewRegistry()
	seen := make(chan string, 2)
	registry.MustRegister("session_probe", tools.Schema{Name: "session_probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		seen <- tools.MCPSessionID(ctx)
		return tools.TextResult("ok"), nil
	})
	runtime := &tools.Runtime{Registry: registry}
	for _, id := range []string{"tunnel_a", "tunnel_b"} {
		bridge, err := newSDKBridgeForTunnel(runtime, id)
		if err != nil {
			t.Fatal(err)
		}
		ctx := ContextWithSessionID(context.Background(), "same-remote-id")
		request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "session_probe", Arguments: json.RawMessage(`{}`)}}
		if _, err := bridge.toolHandler("session_probe")(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-seen, <-seen
	if first == second || first != "tunnel:8:tunnel_a:same-remote-id" || second != "tunnel:8:tunnel_b:same-remote-id" {
		t.Fatalf("session keys %q and %q are not isolated", first, second)
	}
}

func TestTunnelBridgesCallSharedRuntimeConcurrently(t *testing.T) {
	registry := tools.NewRegistry()
	var calls atomic.Int32
	registry.MustRegister("shared_probe", tools.Schema{Name: "shared_probe", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (tools.Result, error) {
		calls.Add(1)
		return tools.TextResult("ok"), nil
	})
	runtime := &tools.Runtime{Registry: registry}
	bridges := make([]*sdkBridge, 0, 2)
	for _, id := range []string{"tunnel_a", "tunnel_b"} {
		bridge, err := newSDKBridgeForTunnel(runtime, id)
		if err != nil {
			t.Fatal(err)
		}
		bridges = append(bridges, bridge)
	}
	start := make(chan struct{})
	errs := make(chan error, len(bridges))
	var wg sync.WaitGroup
	for _, bridge := range bridges {
		wg.Add(1)
		go func(bridge *sdkBridge) {
			defer wg.Done()
			<-start
			ctx := ContextWithSessionID(context.Background(), "same-remote-id")
			result, err := bridge.toolHandler("shared_probe")(ctx, &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "shared_probe", Arguments: json.RawMessage(`{}`)}})
			if err != nil {
				errs <- err
				return
			}
			if result.IsError {
				errs <- errors.New("shared probe returned tool error")
			}
		}(bridge)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("shared runtime calls = %d, want 2", got)
	}
}

func TestTunnelSessionNamespaceIsolatesWorkspaceAccess(t *testing.T) {
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	a, err := workspaces.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := workspaces.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	registry.MustRegister("workspace_probe", tools.Schema{Name: "workspace_probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"}},"required":["workspace_id"]}`), Annotations: tools.ToolAnnotations(tools.RiskRead)}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("ok"), nil
	})
	runtime := &tools.Runtime{Registry: registry, Workspaces: workspaces, SessionAccess: tools.NewSessionWorkspaceAccessManager()}
	for _, test := range []struct {
		id          string
		workspaceID string
	}{{id: "tunnel_a", workspaceID: a.ID}, {id: "tunnel_b", workspaceID: b.ID}} {
		bridge, err := newSDKBridgeForTunnel(runtime, test.id)
		if err != nil {
			t.Fatal(err)
		}
		args, err := json.Marshal(map[string]any{"workspace_id": test.workspaceID})
		if err != nil {
			t.Fatal(err)
		}
		ctx := ContextWithSessionID(context.Background(), "same-remote-id")
		result, err := bridge.toolHandler("workspace_probe")(ctx, &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "workspace_probe", Arguments: args}})
		if err != nil || result.IsError {
			t.Fatalf("%s workspace probe result=%#v err=%v", test.id, result, err)
		}
	}
	accessA, ok := runtime.SessionAccess.Lookup("tunnel:8:tunnel_a:same-remote-id")
	if !ok || len(accessA.Workspaces) != 1 {
		t.Fatalf("tunnel A access = %#v ok=%t", accessA, ok)
	}
	if _, ok := accessA.Workspaces[a.ID]; !ok {
		t.Fatalf("tunnel A missing workspace %s: %#v", a.ID, accessA)
	}
	if _, ok := accessA.Workspaces[b.ID]; ok {
		t.Fatalf("tunnel A inherited tunnel B workspace %s", b.ID)
	}
	accessB, ok := runtime.SessionAccess.Lookup("tunnel:8:tunnel_b:same-remote-id")
	if !ok || len(accessB.Workspaces) != 1 {
		t.Fatalf("tunnel B access = %#v ok=%t", accessB, ok)
	}
	if _, ok := accessB.Workspaces[b.ID]; !ok {
		t.Fatalf("tunnel B missing workspace %s: %#v", b.ID, accessB)
	}
	if _, ok := accessB.Workspaces[a.ID]; ok {
		t.Fatalf("tunnel B inherited tunnel A workspace %s", a.ID)
	}
}

func TestTunnelSessionNamespaceIsolatesLoopGuard(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("project_context", tools.Schema{Name: "project_context", InputSchema: json.RawMessage(`{"type":"object"}`), Annotations: tools.ToolAnnotations(tools.RiskRead)}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("context"), nil
	})
	runtime := &tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()}
	a, err := newSDKBridgeForTunnel(runtime, "tunnel_a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newSDKBridgeForTunnel(runtime, "tunnel_b")
	if err != nil {
		t.Fatal(err)
	}
	call := func(bridge *sdkBridge) (*sdkmcp.CallToolResult, error) {
		ctx := ContextWithSessionID(context.Background(), "same-remote-id")
		return bridge.toolHandler("project_context")(ctx, &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "project_context", Arguments: json.RawMessage(`{}`)}})
	}
	for i := 0; i < 2; i++ {
		result, err := call(a)
		if err != nil || result.IsError {
			t.Fatalf("tunnel A call %d result=%#v err=%v", i, result, err)
		}
	}
	resultB, err := call(b)
	if err != nil || resultB.IsError {
		t.Fatalf("tunnel B inherited tunnel A loop history: result=%#v err=%v", resultB, err)
	}
	blockedA, err := call(a)
	if err != nil || !blockedA.IsError {
		t.Fatalf("tunnel A loop guard did not block third call: result=%#v err=%v", blockedA, err)
	}
}

func TestTunnelSessionNamespaceIsolatesApprovalRetry(t *testing.T) {
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := workspaces.Instance()
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	runtime := &tools.Runtime{Registry: registry, Workspaces: workspaces, SessionAccess: tools.NewSessionWorkspaceAccessManager(), Approvals: approval.NewManager(identity.ID)}
	registry.MustRegister("guarded_action", tools.Schema{Name: "guarded_action", InputSchema: json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"},"command":{"type":"string"}},"required":["workspace_id","command"]}`)}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		if requestID := tools.ApprovalRequestID(ctx); requestID != "" {
			return tools.JSONResult(map[string]any{"approved_request": requestID}), nil
		}
		command, _ := args["command"].(string)
		return tools.Result{}, controlguard.New(controlguard.CodeControlPlaneMutation, "approval required", true, &controlguard.Invocation{Program: "cgm", Args: []string{"update"}, Command: command})
	})
	tools.RegisterApprovalTools(registry, runtime)
	a, err := newSDKBridgeForTunnel(runtime, "tunnel_a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newSDKBridgeForTunnel(runtime, "tunnel_b")
	if err != nil {
		t.Fatal(err)
	}
	call := func(bridge *sdkBridge, name string, args map[string]any) (*sdkmcp.CallToolResult, error) {
		data, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		ctx := ContextWithSessionID(context.Background(), "same-remote-id")
		return bridge.toolHandler(name)(ctx, &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: name, Arguments: data}})
	}
	args := map[string]any{"workspace_id": item.ID, "command": "cgm update"}
	guarded, err := call(a, "guarded_action", args)
	if err != nil || !guarded.IsError {
		t.Fatalf("initial tunnel A result=%#v err=%v", guarded, err)
	}
	body, ok := guarded.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("approval challenge=%#v", guarded.StructuredContent)
	}
	challengeID, _ := body["challenge_id"].(string)
	if challengeID == "" {
		t.Fatalf("approval challenge id missing: %#v", body)
	}
	approvalDone := make(chan error, 1)
	go func() {
		result, err := call(a, tools.ApprovalRequestToolName, map[string]any{"workspace_id": item.ID, "challenge_id": challengeID, "title": "Update ChatGPT MCP"})
		if err == nil && result.IsError {
			err = errors.New("approval request tool returned error")
		}
		approvalDone <- err
	}()
	request := waitForTunnelApproval(t, runtime.Approvals)
	if _, err := runtime.Approvals.Approve(request.ID, "test", "reviewed"); err != nil {
		t.Fatal(err)
	}
	if err := <-approvalDone; err != nil {
		t.Fatal(err)
	}
	other, err := call(b, "guarded_action", args)
	if err != nil || !other.IsError {
		t.Fatalf("tunnel B claimed tunnel A approval: result=%#v err=%v", other, err)
	}
	retry, err := call(a, "guarded_action", args)
	if err != nil || retry.IsError {
		t.Fatalf("tunnel A failed to claim its approval: result=%#v err=%v", retry, err)
	}
	payload, ok := retry.StructuredContent.(map[string]any)
	if !ok || payload["approved_request"] != request.ID {
		t.Fatalf("tunnel A retry payload=%#v", retry.StructuredContent)
	}
}

func TestCollectionValidation(t *testing.T) {
	for _, cfg := range []CollectionConfig{
		{Instances: []InstanceConfig{{ID: "duplicate"}, {ID: "duplicate"}}},
		{Admins: []AdminConfig{{ID: "default"}, {ID: "default"}}},
		{Instances: []InstanceConfig{{ID: "tunnel_a", AdminProfileID: "missing"}}},
		{Instances: []InstanceConfig{{ID: "tunnel_a", Enabled: true}}},
	} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("accepted invalid config: %#v", cfg)
		}
	}
}

func TestCollectionJSONRedactsKeys(t *testing.T) {
	cfg := CollectionConfig{Instances: []InstanceConfig{{ID: "tunnel_a", APIKey: "runtime-secret"}}, Admins: []AdminConfig{{ID: "default", AdminKey: "admin-secret"}}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsSecret(string(data), "runtime-secret", "admin-secret") {
		t.Fatalf("collection JSON exposed a key: %s", data)
	}
}

func containsSecret(data string, secrets ...string) bool {
	for _, secret := range secrets {
		if strings.Contains(data, secret) {
			return true
		}
	}
	return false
}
