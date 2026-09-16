package securemcptunnel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

func TestHandlerDescribe(t *testing.T) {
	desc, err := NewHandler().Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := desc.(runtimeplugin.DescribeResult)
	if result.Provider != tunnelprovider.ProviderSecureMCP || len(result.Targets) != 1 || result.Targets[0].OriginKind != tunnelprovider.OriginPrivate {
		t.Fatalf("describe = %#v", result)
	}
}

func TestHandlerStartInstanceRequiresID(t *testing.T) {
	_, err := invokeHandler(t, testHandler(t), "start_instance", map[string]string{"id": "  "})
	if err == nil || !strings.Contains(err.Error(), "tunnel id is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandlerStartInstanceUnknownID(t *testing.T) {
	_, err := invokeHandler(t, testHandler(t), "start_instance", map[string]string{"id": "missing"})
	if err == nil || !strings.Contains(err.Error(), `tunnel "missing" is not attached`) {
		t.Fatalf("err = %v", err)
	}
}

func TestHandlerStartStopInstanceIsIDScoped(t *testing.T) {
	h := testHandler(t)
	ctx := context.Background()
	if err := h.Manager.Reconcile(ctx, tunnel.CollectionConfig{Instances: []tunnel.InstanceConfig{
		{Enabled: true, ID: "tunnel_a", APIKey: "key-a"},
		{Enabled: true, ID: "tunnel_b", APIKey: "key-b"},
	}}); err != nil {
		t.Fatal(err)
	}
	started, err := invokeHandler(t, h, "start_instance", map[string]string{"id": "tunnel_a"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(started); strings.Contains(string(raw), "key-a") || strings.Contains(string(raw), "key-b") {
		t.Fatalf("status leaked secrets: %s", raw)
	}
	waitReady(t, h, "tunnel_a")
	a := mustStatus(t, h, "tunnel_a")
	b := mustStatus(t, h, "tunnel_b")
	if !a.Running || !a.Ready {
		t.Fatalf("A after start = %+v", a)
	}
	if b.Running || b.Ready {
		t.Fatalf("B changed after A start = %+v", b)
	}
	if _, err := invokeHandler(t, h, "start_instance", map[string]string{"id": "tunnel_b"}); err != nil {
		t.Fatal(err)
	}
	waitReady(t, h, "tunnel_b")
	if _, err := invokeHandler(t, h, "stop_instance", map[string]string{"id": "tunnel_a"}); err != nil {
		t.Fatal(err)
	}
	a = mustStatus(t, h, "tunnel_a")
	b = mustStatus(t, h, "tunnel_b")
	if a.Running || a.Ready {
		t.Fatalf("A after stop = %+v", a)
	}
	if !b.Running || !b.Ready {
		t.Fatalf("B changed after A stop = %+v", b)
	}
}

func TestHandlerRuntimeStatusReportsMixedInstances(t *testing.T) {
	h := testHandler(t)
	ctx := context.Background()
	if err := h.Manager.Reconcile(ctx, tunnel.CollectionConfig{Instances: []tunnel.InstanceConfig{
		{Enabled: true, ID: "tunnel_a", APIKey: "key-a"},
		{Enabled: true, ID: "tunnel_b", APIKey: "key-b"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := invokeHandler(t, h, "start_instance", map[string]string{"id": "tunnel_a"}); err != nil {
		t.Fatal(err)
	}
	waitReady(t, h, "tunnel_a")
	raw, err := invokeHandler(t, h, "runtime_status", nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []tunnel.Status
	if err := json.Unmarshal(encoded, &statuses); err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses = %#v", statuses)
	}
	byID := map[string]tunnel.Status{}
	for _, item := range statuses {
		byID[item.ID] = item
	}
	if !byID["tunnel_a"].Ready || byID["tunnel_b"].Ready {
		t.Fatalf("mixed status = %#v", byID)
	}
}

type stubBackend struct {
	ready chan struct{}
	done  chan os.Signal
}

func (b *stubBackend) Start(context.Context) error {
	select {
	case <-b.ready:
	default:
		close(b.ready)
	}
	return nil
}
func (b *stubBackend) Stop(context.Context) error { return nil }
func (b *stubBackend) WaitUntilReady(ctx context.Context) error {
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b *stubBackend) Done() <-chan os.Signal { return b.done }

func testHandler(t *testing.T) *Handler {
	t.Helper()
	h := NewHandler()
	h.Manager.SetBackendFactory(func(tunnel.Config, sdkmcp.Transport) (tunnel.Backend, error) {
		return &stubBackend{ready: make(chan struct{}), done: make(chan os.Signal)}, nil
	})
	h.Manager.SetBridge("http://127.0.0.1:9/mcp", "token")
	return h
}

func invokeHandler(t *testing.T, h *Handler, op string, payload any) (any, error) {
	t.Helper()
	raw, err := json.Marshal(runtimeplugin.InvokeParams{Op: op, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return h.Invoke(context.Background(), raw)
}

func mustStatus(t *testing.T, h *Handler, id string) tunnel.Status {
	t.Helper()
	status, err := h.instanceStatus(id)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func waitReady(t *testing.T, h *Handler, id string) {
	t.Helper()
	client, ok := h.Manager.Client(id)
	if !ok {
		t.Fatalf("tunnel %q missing", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.WaitUntilReady(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerUnknownInvoke(t *testing.T) {
	_, err := invokeHandler(t, NewHandler(), "nope", nil)
	if !errors.Is(err, runtimeplugin.ErrUnknownMethod) {
		t.Fatalf("err = %v", err)
	}
}
