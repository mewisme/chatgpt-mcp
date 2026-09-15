package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/tunnel-client/pkg/tunnelctx"

	"go.mewis.me/chatgpt-mcp/internal/tools"
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

func TestManagerStartsHealthyTunnelWhenAnotherFails(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	m := NewManager(runtime, nil)
	m.factory = func(cfg Config, _ sdkmcp.Transport) (backend, error) {
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
	m.factory = func(cfg Config, _ sdkmcp.Transport) (backend, error) {
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
		ctx := tunnelctx.ContextWithSessionID(context.Background(), "same-remote-id")
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
