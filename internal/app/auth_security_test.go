package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestDirectMCPHTTPCredentialsStayIsolated(t *testing.T) {
	testutil.UseConfigRoot(t, filepath.Join(t.TempDir(), "config"))
	result, err := application.Initialize(application.InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := result.Config
	cfg.Tunnel.Enabled = true
	cfg.Tunnel.ID = "tunnel_test"
	cfg.Tunnel.APIKey = "sk-runtime-not-mcp"
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	mcpReveal := httptest.NewRecorder()
	mcpReq := httptest.NewRequest(http.MethodGet, "/api/auth/mcp-token", nil)
	mcpReq.Header.Set("Authorization", "Bearer "+result.MCPToken)
	app.MCPHandler().ServeHTTP(mcpReveal, mcpReq)
	if mcpReveal.Code != http.StatusNotFound {
		t.Fatalf("MCP handler served admin reveal: %d %s", mcpReveal.Code, mcpReveal.Body.String())
	}

	for _, token := range []string{result.AdminToken, cfg.Tunnel.APIKey, ""} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		app.MCPHandler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("/mcp accepted non-MCP token %q: %d", token, recorder.Code)
		}
	}

	accepted := httptest.NewRecorder()
	okReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	okReq.Header.Set("Authorization", "Bearer "+result.MCPToken)
	app.MCPHandler().ServeHTTP(accepted, okReq)
	if accepted.Code == http.StatusUnauthorized {
		t.Fatal("/mcp rejected Direct MCP HTTP token")
	}

	rejected := httptest.NewRecorder()
	badAdmin := httptest.NewRequest(http.MethodGet, "/api/auth/mcp-token", nil)
	badAdmin.Header.Set("Authorization", "Bearer "+result.MCPToken)
	app.AdminHandler().ServeHTTP(rejected, badAdmin)
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("admin reveal accepted Direct MCP HTTP token: %d %s", rejected.Code, rejected.Body.String())
	}

	revealed := httptest.NewRecorder()
	adminReq := httptest.NewRequest(http.MethodGet, "/api/auth/mcp-token", nil)
	adminReq.Header.Set("Authorization", "Bearer "+result.AdminToken)
	app.AdminHandler().ServeHTTP(revealed, adminReq)
	if revealed.Code != http.StatusOK || !strings.Contains(revealed.Body.String(), result.MCPToken) {
		t.Fatalf("admin reveal=%d %s", revealed.Code, revealed.Body.String())
	}

	configView := httptest.NewRecorder()
	cfgReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	cfgReq.Header.Set("Authorization", "Bearer "+result.AdminToken)
	app.AdminHandler().ServeHTTP(configView, cfgReq)
	body := configView.Body.String()
	if configView.Code != http.StatusOK || strings.Contains(body, result.MCPToken) || strings.Contains(body, result.AdminToken) {
		t.Fatalf("config leaked token: %d %s", configView.Code, body)
	}

	for _, schema := range app.Tools.List() {
		name := strings.ToLower(schema.Name)
		if strings.Contains(name, "oauth") || strings.Contains(name, "mcp-token") || strings.Contains(name, "mcp_token") {
			t.Fatalf("MCP tool surface exposes auth endpoint %q", schema.Name)
		}
	}
}
