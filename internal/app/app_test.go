package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestNewDoesNotOwnLiveSecureMCPManager(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Enabled = true
	cfg.Tunnel.Enabled = true
	cfg.Tunnel.ID = "tunnel_test"
	cfg.Tunnel.APIKey = "runtime-secret"
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if app.Tunnels != nil || app.Tunnel != nil {
		t.Fatal("New constructed a live Secure MCP manager")
	}
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Stop() })
	if app.Tunnels != nil || app.Tunnel != nil {
		t.Fatal("Start constructed a live Secure MCP manager")
	}
}

func TestNewSharesToolRuntime(t *testing.T) {
	cfg := config.Default()
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
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

func TestTunnelOnlyRuntimeDoesNotCreateMCPHTTPRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Enabled = false
	cfg.Tunnel.Enabled = true
	cfg.Tunnel.ID = "tunnel_test"
	cfg.Tunnel.APIKey = "runtime-secret"
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if app.MCP != nil || app.Tools == nil || app.Tunnels != nil || app.Tunnel != nil {
		t.Fatalf("tunnel-only runtime MCP=%#v tools=%#v tunnels=%#v tunnel=%#v", app.MCP, app.Tools, app.Tunnels, app.Tunnel)
	}
	recorder := httptest.NewRecorder()
	app.MCPHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled MCP HTTP status=%d", recorder.Code)
	}
}

func TestReloadConfigSwitchesMCPHTTPRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	next := cfg
	next.Server.Enabled = false
	next.Tunnel.Enabled = true
	next.Tunnel.ID = "tunnel_test"
	next.Tunnel.APIKey = "runtime-secret"
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if app.MCP != nil {
		t.Fatal("MCP HTTP runtime survived transport disable")
	}
	next.Server.Enabled = true
	next.Tunnel.Enabled = false
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if app.MCP == nil || app.MCP.Server == nil || app.MCP.Server.Tools != app.Tools {
		t.Fatal("MCP HTTP runtime was not restored with shared tools")
	}
}

func TestNewKeepsControllerToolsWhenFeatureInactive(t *testing.T) {
	cfg := config.Default()
	store := pluginpkg.SettingsStore{Layout: pluginpkg.DefaultLayout()}
	schema := pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
		{Key: "default_active", Kind: pluginpkg.FieldBool, Default: true},
		{Key: "default_mode", Kind: pluginpkg.FieldEnum, Enum: []string{"lite", "full", "ultra"}, Default: "full"},
	}}
	if err := store.Set(schema, "ponytail", "default_active", false); err != nil {
		t.Fatal(err)
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if app.Tools == nil {
		t.Fatal("tool runtime missing")
	}
}

func TestHandlersHonorDisabledAuthentication(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	mcpRecorder := httptest.NewRecorder()
	app.MCPHandler().ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if mcpRecorder.Code != http.StatusOK {
		t.Fatalf("MCP auth-disabled health = %d", mcpRecorder.Code)
	}
	if body := mcpRecorder.Body.String(); body != `{"ok":true}` || strings.Contains(body, "auth_enabled") {
		t.Fatalf("MCP health body = %q", body)
	}

	adminRecorder := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin auth-disabled health = %d", adminRecorder.Code)
	}
}

func TestAdminAPIAvailableWithoutAdminUIPlugin(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	handler := app.AdminHandler()
	apiRecorder := httptest.NewRecorder()
	handler.ServeHTTP(apiRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if apiRecorder.Code != http.StatusOK {
		t.Fatalf("admin API without UI plugin = %d", apiRecorder.Code)
	}
	uiRecorder := httptest.NewRecorder()
	handler.ServeHTTP(uiRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if uiRecorder.Code != http.StatusServiceUnavailable || !strings.Contains(uiRecorder.Body.String(), "cgm plugin install admin-ui") {
		t.Fatalf("admin UI without plugin = %d %q", uiRecorder.Code, uiRecorder.Body.String())
	}
}

func TestHandlersRequireEnabledAuthentication(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = auth.HashToken("mcp-test")
	cfg.Auth.AdminTokenHash = auth.HashToken("admin-test")
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	mcpRecorder := httptest.NewRecorder()
	app.MCPHandler().ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if mcpRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("MCP auth-enabled status = %d", mcpRecorder.Code)
	}

	adminRecorder := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(adminRecorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("admin health should stay public = %d", adminRecorder.Code)
	}
	protected := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("admin auth-enabled status = %d", protected.Code)
	}
}

func TestMCPHandlerOmitsInboundOAuthDiscovery(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp", "/oauth/register", "/oauth/authorize", "/oauth/token"} {
		recorder := httptest.NewRecorder()
		app.MCPHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, recorder.Code)
		}
	}
}

func TestHandlersReadAuthenticationFromRuntimeConfigStore(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = auth.HashToken("mcp-test")
	cfg.Auth.AdminTokenHash = auth.HashToken("admin-test")
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mcpHandler := app.MCPHandler()
	adminHandler := app.AdminHandler()

	if _, err := app.Config.Update(func(next config.Config) (config.Config, error) {
		next.Auth.MCPEnabled = false
		next.Auth.AdminEnabled = false
		next.Server.AllowUnauthenticatedLoopback = true
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
	if app.Activity == nil || app.Logger == nil || app.MCP.Activity != app.Activity {
		t.Fatal("bootstrap did not wire runtime telemetry")
	}
}

func TestAdminHandlerSharesApprovalManager(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.AdminEnabled = false
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, err := app.Tools.Approvals.CreateChallenge(approval.ChallengeInput{SessionID: "session-a", SessionHash: "hash-a", WorkspaceID: "ws_test", Source: "tunnel", TargetTool: "run_command", Arguments: map[string]any{"workspace_id": "ws_test", "command": "cgm update"}, GuardCode: controlguard.CodeControlPlaneMutation, GuardReason: "guarded", Title: "Allow cgm update"})
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := app.Tools.Approvals.CreateRequest(challenge.ID, "session-a", "ws_test")
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodGet, "/api/requests?status=pending", nil)
	httpRequest.RemoteAddr = "127.0.0.1:43123"
	recorder := httptest.NewRecorder()
	app.AdminHandler().ServeHTTP(recorder, httpRequest)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), request.ID) {
		t.Fatalf("approval API status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPRuntimeStartsWhenTunnelIsUnconfigured(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel.Enabled = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if app.MCP == nil {
		t.Fatal("HTTP MCP runtime unavailable")
	}
	if err := app.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAppLifecycleOwnsWorkspaceRuntimeLock(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	cfg := config.Default()
	cfg.Tunnel.Enabled = false
	first, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Tools.Workspaces.Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	second, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); !errors.Is(err, workspace.ErrAlreadyActive) {
		t.Fatalf("second app start error=%v", err)
	}
	if err := first.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("second app start after release failed: %v", err)
	}
	if err := second.Stop(); err != nil {
		t.Fatal(err)
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
	if err := manager.Add(upstream.Server{ID: "one", Name: "One", Enabled: true, Transport: "http", URL: "https://example.test/mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := (&App{Upstream: manager}).Stop(); err != nil {
		t.Fatal(err)
	}
	if len(client.closed) != 1 || client.closed[0] != "one" {
		t.Fatalf("closed = %#v", client.closed)
	}
}
