package tunnel

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

type fakeBackend struct {
	mu      sync.Mutex
	started bool
	stopped bool
	ready   chan struct{}
	done    chan os.Signal
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{ready: make(chan struct{}), done: make(chan os.Signal)}
}

func (b *fakeBackend) Start(context.Context) error {
	b.mu.Lock()
	b.started = true
	b.mu.Unlock()
	close(b.ready)
	return nil
}

func (b *fakeBackend) Stop(context.Context) error {
	b.mu.Lock()
	b.stopped = true
	b.mu.Unlock()
	return nil
}

func (b *fakeBackend) WaitUntilReady(ctx context.Context) error {
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *fakeBackend) Done() <-chan os.Signal { return b.done }

func TestDisabledTunnelDoesNotConstructBackend(t *testing.T) {
	called := false
	client := newConfigured(Config{Enabled: false}, &tools.Runtime{Registry: tools.NewRegistry()}, func(Config, sdkmcp.Transport) (backend, error) {
		called = true
		return newFakeBackend(), nil
	})
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	if called || client.Status().Running {
		t.Fatalf("disabled tunnel constructed backend: called=%v status=%+v", called, client.Status())
	}
}

func TestBuiltinOpenAITunnelLifecycle(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	fake := newFakeBackend()
	var seen Config
	client := newConfigured(Config{
		Enabled: true, ID: "tunnel_0123456789abcdef0123456789abcdef", APIKey: "secret",
		ControlPlaneBaseURL: "https://api.openai.com", OrganizationID: "org_test",
	}, runtime, func(cfg Config, transport sdkmcp.Transport) (backend, error) {
		if transport == nil {
			t.Fatal("expected in-memory MCP transport")
		}
		seen = cfg
		return fake, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), time.Second)
	defer readyCancel()
	if err := client.WaitUntilReady(readyCtx); err != nil {
		t.Fatal(err)
	}
	status := client.Status()
	if status.Provider != ProviderOpenAI || !status.Running || !status.Ready || status.ID != seen.ID {
		t.Fatalf("status = %+v", status)
	}
	if err := client.Stop(); err != nil {
		t.Fatal(err)
	}
	if status = client.Status(); status.Running || status.Ready {
		t.Fatalf("expected stopped tunnel: %+v", status)
	}
	fake.mu.Lock()
	stopped := fake.stopped
	fake.mu.Unlock()
	if !stopped {
		t.Fatal("embedded OpenAI backend was not stopped")
	}
}

func TestTunnelContextCancellationStopsEmbeddedBackend(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	fake := newFakeBackend()
	client := newConfigured(Config{Enabled: true, ID: "tunnel_test", APIKey: "secret"}, runtime, func(Config, sdkmcp.Transport) (backend, error) { return fake, nil })
	ctx, cancel := context.WithCancel(context.Background())
	if err := client.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for client.Status().Running {
		if time.Now().After(deadline) {
			t.Fatalf("tunnel did not stop after context cancellation: %+v", client.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	fake.mu.Lock()
	stopped := fake.stopped
	fake.mu.Unlock()
	if !stopped {
		t.Fatal("embedded backend was not stopped after context cancellation")
	}
}

func TestEnabledTunnelRequiresRuntimeIDAndAPIKey(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		runtime *tools.Runtime
	}{
		{name: "runtime", cfg: Config{Enabled: true, ID: "tunnel_test", APIKey: "secret"}},
		{name: "id", cfg: Config{Enabled: true, APIKey: "secret"}, runtime: &tools.Runtime{Registry: tools.NewRegistry()}},
		{name: "api-key", cfg: Config{Enabled: true, ID: "tunnel_test"}, runtime: &tools.Runtime{Registry: tools.NewRegistry()}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := newConfigured(test.cfg, test.runtime, func(Config, sdkmcp.Transport) (backend, error) { return newFakeBackend(), nil })
			if err := client.Start(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestConfigureRejectsRunningTunnel(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry()}
	client := newConfigured(Config{Enabled: true, ID: "tunnel_test", APIKey: "secret"}, runtime, func(Config, sdkmcp.Transport) (backend, error) { return newFakeBackend(), nil })
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer client.Stop()
	if err := client.Configure(Config{}); err == nil {
		t.Fatal("expected running configure rejection")
	}
}
