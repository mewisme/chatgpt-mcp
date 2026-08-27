package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelConfigRedactsAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel = tunnel.Config{Enabled: true, APIKey: "secret", Command: "tunnel"}
	handler := New(API{Tunnel: tunnel.NewConfigured(cfg.Tunnel), Config: &cfg})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/tunnel/config", nil))
	if recorder.Code != 200 {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("API key must not be returned")
	}
}
