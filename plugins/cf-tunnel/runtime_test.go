package cftunnel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/pkg/cloudflared"
)

func TestManagerDisabledNeverStarts(t *testing.T) {
	var started atomic.Int32
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		started.Add(1)
		return stubTunnel(ctx, cfg), nil
	})
	m.Sync(context.Background(), Snapshot{PluginEnabled: false, DesiredMCP: true, DesiredAdmin: true, MCP: readyEndpoint(37421), Admin: readyEndpoint(37422)})
	time.Sleep(20 * time.Millisecond)
	if started.Load() != 0 {
		t.Fatalf("started = %d", started.Load())
	}
}

func TestManagerAuthGatePerTarget(t *testing.T) {
	var mu sync.Mutex
	var origins []string
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		mu.Lock()
		origins = append(origins, cfg.OriginURL)
		mu.Unlock()
		return stubTunnel(ctx, cfg), nil
	})
	m.Sync(context.Background(), Snapshot{
		PluginEnabled: true, DesiredMCP: true, DesiredAdmin: true,
		MCP:   Endpoint{Ready: true, Port: 37421, AuthErr: ErrMCPAuthDisabled},
		Admin: readyEndpoint(37422),
	})
	admin := waitTarget(t, m, TargetAdmin, true, "")
	if admin.Origin != "http://127.0.0.1:37422" {
		t.Fatalf("admin origin = %q", admin.Origin)
	}
	mcp := targetStatus(m, TargetMCP)
	if mcp.LastError != ErrMCPAuthDisabled.Error() || mcp.Ready {
		t.Fatalf("mcp = %#v", mcp)
	}
	mu.Lock()
	got := append([]string(nil), origins...)
	mu.Unlock()
	if len(got) != 1 || got[0] != admin.Origin {
		t.Fatalf("origins = %#v", got)
	}
}

func TestManagerIndependentTargetsAndPortRestart(t *testing.T) {
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		return stubTunnel(ctx, cfg), nil
	})
	m.Sync(context.Background(), Snapshot{PluginEnabled: true, DesiredMCP: true, DesiredAdmin: true, MCP: readyEndpoint(37421), Admin: readyEndpoint(37422)})
	mcp := waitTarget(t, m, TargetMCP, true, "")
	admin := waitTarget(t, m, TargetAdmin, true, "")
	if mcp.URL == admin.URL || mcp.Origin != "http://127.0.0.1:37421" {
		t.Fatalf("mcp=%#v admin=%#v", mcp, admin)
	}
	keepAdmin := admin.URL
	m.Sync(context.Background(), Snapshot{PluginEnabled: true, DesiredMCP: true, DesiredAdmin: true, MCP: readyEndpoint(38001), Admin: readyEndpoint(37422)})
	mcp = waitTarget(t, m, TargetMCP, true, "")
	admin = waitTarget(t, m, TargetAdmin, true, "")
	if mcp.Origin != "http://127.0.0.1:38001" {
		t.Fatalf("mcp origin = %q", mcp.Origin)
	}
	if admin.URL != keepAdmin {
		t.Fatalf("admin url rotated: %q -> %q", keepAdmin, admin.URL)
	}
}

func TestManagerOneFailureDoesNotStopTheOther(t *testing.T) {
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		if strings.Contains(cfg.OriginURL, ":37421") {
			return nil, errors.New("edge down")
		}
		return stubTunnel(ctx, cfg), nil
	})
	m.Sync(context.Background(), Snapshot{PluginEnabled: true, DesiredMCP: true, DesiredAdmin: true, MCP: readyEndpoint(37421), Admin: readyEndpoint(37422)})
	waitTarget(t, m, TargetAdmin, true, "")
	mcp := waitTarget(t, m, TargetMCP, false, "edge down")
	if mcp.Running {
		t.Fatalf("mcp still running: %#v", mcp)
	}
}

func readyEndpoint(port int) Endpoint {
	return Endpoint{Ready: true, Port: port}
}

func TestManagerReconnectsAfterUnexpectedWait(t *testing.T) {
	var started atomic.Int32
	var mu sync.Mutex
	var tunnels []*cloudflared.Tunnel
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		started.Add(1)
		tun := stubTunnel(ctx, cfg)
		mu.Lock()
		tunnels = append(tunnels, tun)
		mu.Unlock()
		return tun, nil
	})
	m.Sync(context.Background(), Snapshot{PluginEnabled: true, DesiredMCP: true, MCP: readyEndpoint(37421)})
	waitTarget(t, m, TargetMCP, true, "")
	mu.Lock()
	first := tunnels[0]
	mu.Unlock()
	first.Complete(errors.New("edge down"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := targetStatus(m, TargetMCP)
		if started.Load() >= 2 && st.Ready && st.LastError == "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("started=%d status=%#v", started.Load(), targetStatus(m, TargetMCP))
}

func TestManagerRedactsSecretsAndEmitsLifecycle(t *testing.T) {
	var mu sync.Mutex
	var events []LifecycleEvent
	m := newTestManager(t, func(ctx context.Context, cfg cloudflared.Config) (*cloudflared.Tunnel, error) {
		return nil, errors.New("provision failed secret=cf-secret-value token=mcp_abcdefghijklmnopqrstuvwxyz012345")
	})
	m.SetObserver(func(event LifecycleEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	m.Sync(context.Background(), Snapshot{PluginEnabled: true, DesiredMCP: true, MCP: readyEndpoint(37421)})
	deadline := time.Now().Add(2 * time.Second)
	var st TargetStatus
	for time.Now().Before(deadline) {
		st = targetStatus(m, TargetMCP)
		if !st.Ready && st.LastError != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if st.LastError == "" {
		t.Fatalf("missing last error: %#v", st)
	}
	if strings.Contains(st.LastError, "cf-secret-value") || strings.Contains(st.LastError, "mcp_abcdefghijklmnopqrstuvwxyz012345") {
		t.Fatalf("secret leaked in status: %#v", st)
	}
	if !strings.Contains(st.LastError, "<redacted>") {
		t.Fatalf("last error not redacted: %#v", st)
	}
	mu.Lock()
	got := append([]LifecycleEvent(nil), events...)
	mu.Unlock()
	if len(got) < 2 || got[0].State != LifecycleConnecting || got[len(got)-1].State != LifecycleDegraded {
		t.Fatalf("events = %#v", got)
	}
	for _, event := range got {
		if strings.Contains(event.Error, "cf-secret-value") || strings.Contains(event.URL, "cf-secret-value") {
			t.Fatalf("secret leaked in event: %#v", event)
		}
	}
}

func TestTargetStatusLineReconnects(t *testing.T) {
	if line := (TargetStatus{Restarting: true, LastError: "edge down"}).Line(); line != "reconnecting · edge down" {
		t.Fatalf("line = %q", line)
	}
}

func newTestManager(t *testing.T, start StartFunc) *Manager {
	t.Helper()
	m := NewManager()
	m.reconnectDelay = 0
	m.SwapStart(start)
	t.Cleanup(m.Stop)
	return m
}

func stubTunnel(ctx context.Context, cfg cloudflared.Config) *cloudflared.Tunnel {
	tun := cloudflared.Stub("https://" + strings.TrimPrefix(cfg.OriginURL, "http://") + ".trycloudflare.test")
	go func() {
		<-ctx.Done()
		tun.Complete(ctx.Err())
	}()
	return tun
}

func waitTarget(t *testing.T, m *Manager, target string, ready bool, lastError string) TargetStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var st TargetStatus
	for time.Now().Before(deadline) {
		st = targetStatus(m, target)
		if st.Ready == ready && st.LastError == lastError {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("target %s = %#v want ready=%t error=%q", target, st, ready, lastError)
	return st
}

func targetStatus(m *Manager, target string) TargetStatus {
	for _, item := range m.Status().Targets {
		if item.Target == target {
			return item
		}
	}
	return TargetStatus{Target: target}
}
