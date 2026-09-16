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

func newTestManager(t *testing.T, start StartFunc) *Manager {
	t.Helper()
	m := NewManager()
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
