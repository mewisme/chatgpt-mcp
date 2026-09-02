package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestNewSharesToolRuntime(t *testing.T) {
	cfg := config.Default()
	app := New(cfg)
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("app runtime was not initialized")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("MCP and Admin do not share the same tool runtime")
	}
	if app.Upstream != app.Tools.Upstream {
		t.Fatal("Admin and tool runtime do not share the same upstream manager")
	}
	if _, ok := app.Tools.Registry.Schema("ponytail_turn"); !ok {
		t.Fatal("default app missing ponytail feature tool")
	}
	if _, ok := app.Tools.Registry.Schema("caveman_turn"); !ok {
		t.Fatal("default app missing caveman feature tool")
	}
}

func TestNewHonorsDisabledFeatures(t *testing.T) {
	cfg := config.Default()
	cfg.Features.Ponytail.Enabled = false
	app := New(cfg)
	if _, ok := app.Tools.Registry.Schema("ponytail_turn"); ok {
		t.Fatal("disabled ponytail tool registered")
	}
	if _, ok := app.Tools.Registry.Schema("caveman_turn"); !ok {
		t.Fatal("enabled caveman tool missing")
	}
}

func TestHandlersHonorDisabledAuthentication(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	app := New(cfg)

	mcpRecorder := httptest.NewRecorder()
	app.MCPHandler().ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if mcpRecorder.Code != http.StatusOK {
		t.Fatalf("MCP auth-disabled health = %d", mcpRecorder.Code)
	}

	adminRecorder := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin auth-disabled health = %d", adminRecorder.Code)
	}
}

func TestHandlersRequireEnabledAuthentication(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = auth.HashToken("mcp-test")
	cfg.Auth.AdminTokenHash = auth.HashToken("admin-test")
	app := New(cfg)

	mcpRecorder := httptest.NewRecorder()
	app.MCPHandler().ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if mcpRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("MCP auth-enabled status = %d", mcpRecorder.Code)
	}

	adminRecorder := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if adminRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("admin auth-enabled status = %d", adminRecorder.Code)
	}
}

func TestHandlersReadAuthenticationFromRuntimeConfigStore(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = auth.HashToken("mcp-test")
	cfg.Auth.AdminTokenHash = auth.HashToken("admin-test")
	app := New(cfg)
	mcpHandler := app.MCPHandler()
	adminHandler := app.AdminHandler()

	if _, err := app.Config.Update(func(next config.Config) (config.Config, error) {
		next.Auth.MCPEnabled = false
		next.Auth.AdminEnabled = false
		return next, nil
	}); err != nil {
		t.Fatal(err)
	}
	mcpRecorder := httptest.NewRecorder()
	mcpHandler.ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if mcpRecorder.Code != http.StatusOK {
		t.Fatalf("updated MCP auth status = %d", mcpRecorder.Code)
	}
	adminRecorder := httptest.NewRecorder()
	adminHandler.ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("updated admin auth status = %d", adminRecorder.Code)
	}
}

func TestBootstrapRewiresToolRuntime(t *testing.T) {
	app := &App{}
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if app.Config == nil {
		t.Fatal("bootstrap did not initialize runtime config store")
	}
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("bootstrap did not initialize runtime")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("bootstrap did not wire shared tool runtime")
	}
	if app.Upstream != app.Tools.Upstream {
		t.Fatal("bootstrap did not wire shared upstream manager")
	}
}

func TestTunnelLifecyclePublishesActivityFromSourceObserver(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel.Enabled = true
	app := New(cfg)
	if err := app.Start(context.Background()); err == nil {
		t.Fatal("expected invalid tunnel configuration to fail")
	}
	recent := app.Activity.Recent(10)
	if len(recent) == 0 {
		t.Fatal("tunnel lifecycle failure did not publish activity")
	}
	last := recent[len(recent)-1]
	if last.Kind != "tunnel.degraded" || last.Status != "degraded" || last.Source != "tunnel" {
		t.Fatalf("activity = %#v", last)
	}
}

type lifecycleUpstreamClient struct {
	closed []string
}

func (*lifecycleUpstreamClient) Connect(context.Context, upstream.Server) error { return nil }
func (c *lifecycleUpstreamClient) Close(_ context.Context, id string) error {
	c.closed = append(c.closed, id)
	return nil
}
func (*lifecycleUpstreamClient) Tools(context.Context, string) ([]upstream.Tool, error) {
	return nil, nil
}
func (*lifecycleUpstreamClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return upstream.CallResult{}, nil
}
func (*lifecycleUpstreamClient) PID(string) int { return 0 }

func TestStopShutsDownUpstreamConnections(t *testing.T) {
	client := &lifecycleUpstreamClient{}
	manager := upstream.NewManagerWithClient(nil, client)
	if err := manager.Add(upstream.Server{ID: "one", Name: "One", Enabled: true, Transport: "http", URL: "http://example.test/mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := (&App{Upstream: manager}).Stop(); err != nil {
		t.Fatal(err)
	}
	if len(client.closed) != 1 || client.closed[0] != "one" {
		t.Fatalf("closed = %#v", client.closed)
	}
}

type failingClusterTransport struct{ err error }

func (t failingClusterTransport) Connect(context.Context, cluster.Advertisement) (cluster.Session, error) {
	return nil, t.err
}

func clusterTestConfig() config.Config {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	return cfg
}

func TestStartConnectsConfiguredClusterAndAdvertisesRuntime(t *testing.T) {
	cfg := clusterTestConfig()
	cfg.Cluster = config.ClusterConfig{Enabled: true, RelayURL: "ws://127.0.0.1:8080/cluster", RelayToken: "secret"}
	relay := cluster.NewMemoryRelay()
	app := New(cfg)
	app.clusterTransportFactory = func(config.ClusterConfig) cluster.Transport { return relay }
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop()
	node := app.Tools.ClusterNode()
	if node == nil {
		t.Fatal("cluster node was not connected")
	}
	snapshot, err := node.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	advertisement, err := app.Tools.ClusterAdvertisement()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 1 || snapshot.Members[0].InstanceID != advertisement.InstanceID || snapshot.Members[0].CatalogHash != advertisement.CatalogHash {
		t.Fatalf("cluster snapshot = %#v advertisement = %#v", snapshot, advertisement)
	}
}

func TestReloadConfigEnablesAndDisablesCluster(t *testing.T) {
	cfg := clusterTestConfig()
	relay := cluster.NewMemoryRelay()
	app := New(cfg)
	app.clusterTransportFactory = func(config.ClusterConfig) cluster.Transport { return relay }
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop()

	next := cfg
	next.Cluster = config.ClusterConfig{Enabled: true, RelayURL: "ws://127.0.0.1:8080/cluster", RelayToken: "secret"}
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if app.Tools.ClusterNode() == nil {
		t.Fatal("cluster node was not started by reload")
	}

	disabled := next
	disabled.Cluster = config.ClusterConfig{}
	if err := app.ReloadConfig(disabled); err != nil {
		t.Fatal(err)
	}
	if app.Tools.ClusterNode() != nil {
		t.Fatal("cluster node remained connected after disabling cluster")
	}
}

func TestReloadConfigRollsBackFailedClusterRelayChange(t *testing.T) {
	cfg := clusterTestConfig()
	cfg.Cluster = config.ClusterConfig{Enabled: true, RelayURL: "ws://127.0.0.1:8080/cluster", RelayToken: "secret"}
	relay := cluster.NewMemoryRelay()
	app := New(cfg)
	app.clusterTransportFactory = func(value config.ClusterConfig) cluster.Transport {
		if value.RelayURL == "wss://broken.example/cluster" {
			return failingClusterTransport{err: errors.New("relay unavailable")}
		}
		return relay
	}
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer app.Stop()

	next := cfg
	next.Cluster.RelayURL = "wss://broken.example/cluster"
	if err := app.ReloadConfig(next); err == nil {
		t.Fatal("expected cluster relay reload failure")
	}
	if got := app.Config.Snapshot().Cluster.RelayURL; got != cfg.Cluster.RelayURL {
		t.Fatalf("persisted runtime cluster URL = %q", got)
	}
	node := app.Tools.ClusterNode()
	if node == nil {
		t.Fatal("previous cluster node was not restored")
	}
	snapshot, err := node.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 1 || !snapshot.Members[0].Online {
		t.Fatalf("restored cluster snapshot = %#v", snapshot)
	}
}
