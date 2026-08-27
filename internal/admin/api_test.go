package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestTunnelConfigRedactsAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel = tunnel.Config{Enabled: true, APIKey: "secret", Command: "tunnel"}
	handler := New(API{Tunnel: tunnel.NewConfigured(cfg.Tunnel), Config: &cfg})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel/config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("API key must not be returned")
	}
}

func TestConfigAPIHidesTokenHashes(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret-hash"
	cfg.Auth.AdminTokenHash = "admin-secret-hash"
	recorder := httptest.NewRecorder()
	New(API{Config: &cfg}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.Contains(body, "secret-hash") || strings.Contains(body, "token_hash") {
		t.Fatalf("config API leaked token hashes: %s", body)
	}
}

func TestUpstreamAPIMutations(t *testing.T) {
	manager := upstream.NewManager(upstream.NewStore(filepath.Join(t.TempDir(), "upstream.json")))
	handler := New(API{Upstream: manager})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/upstream", strings.NewReader(`{"id":"server-1","name":"Server","transport":"http","enabled":true}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("add status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(manager.List()) != 1 {
		t.Fatalf("server was not added: %+v", manager.List())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/upstream/server-1", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(manager.List()) != 0 {
		t.Fatalf("server was not removed: %+v", manager.List())
	}
}
