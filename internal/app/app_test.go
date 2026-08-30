package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestNewSharesToolRuntime(t *testing.T) {
	app := New(config.Config{})
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("app runtime was not initialized")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("MCP and Admin do not share the same tool runtime")
	}
	if app.Upstream != app.Tools.Upstream {
		t.Fatal("Admin and tool runtime do not share the same upstream manager")
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
