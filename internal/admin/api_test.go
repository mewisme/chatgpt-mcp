package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type adminUpstreamClient struct {
	tools []upstream.Tool
}

func (*adminUpstreamClient) Connect(context.Context, upstream.Server) error { return nil }
func (*adminUpstreamClient) Close(context.Context, string) error            { return nil }
func (c *adminUpstreamClient) Tools(context.Context, string) ([]upstream.Tool, error) {
	return append([]upstream.Tool(nil), c.tools...), nil
}
func (*adminUpstreamClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return upstream.CallResult{}, nil
}
func (*adminUpstreamClient) PID(string) int { return 0 }

func TestTunnelConfigRedactsAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel = tunnel.Config{Enabled: true, ID: "tunnel_test", APIKey: "secret"}
	handler := New(API{Tunnel: tunnel.NewConfigured(cfg.Tunnel, nil), Config: &cfg})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel/config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("API key must not be returned")
	}
	if !strings.Contains(recorder.Body.String(), `"id":"tunnel_test"`) {
		t.Fatalf("tunnel id missing: %s", recorder.Body.String())
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
	if !strings.Contains(body, `"mcp_token_configured":true`) || !strings.Contains(body, `"admin_token_configured":true`) {
		t.Fatalf("configured state missing: %s", body)
	}
	if strings.Contains(body, `"host"`) || !strings.Contains(body, `"expose":{"mode":"none","interfaces":[]}`) {
		t.Fatalf("server exposure view is invalid: %s", body)
	}
}

func TestHealthReportsAdminAuthState(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.AdminEnabled = false
	recorder := httptest.NewRecorder()
	New(API{Config: &cfg}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"auth_enabled":false`) {
		t.Fatalf("health = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestWorkspaceAPICRUD(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	root := t.TempDir()
	handler := New(API{Workspaces: manager})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/workspaces", strings.NewReader(`{"path":`+jsonString(root)+`}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("register status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var item workspace.Workspace
	if err := json.Unmarshal(recorder.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.Path == "" {
		t.Fatalf("workspace = %#v", item)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), item.ID) {
		t.Fatalf("list = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/workspaces/"+item.ID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("show = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+item.ID, nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := manager.Get(item.ID); err == nil {
		t.Fatal("workspace still registered")
	}
}

func TestUpstreamAPIManagementAndRedaction(t *testing.T) {
	client := &adminUpstreamClient{tools: []upstream.Tool{{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object"}}}}
	manager := upstream.NewManagerWithClient(nil, client)
	handler := New(API{Upstream: manager})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/upstream", strings.NewReader(`{"id":"server-1","name":"Server","transport":"http","url":"http://example.test/mcp","enabled":true,"headers":{"Authorization":"Bearer secret","X-Test":"ok"},"env":{"API_TOKEN":"secret","MODE":"test"}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("add status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "Bearer secret") || strings.Contains(recorder.Body.String(), `"API_TOKEN":"secret"`) {
		t.Fatalf("server response leaked secrets: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/upstream/server-1/status?refresh=true", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"health":"connected"`) {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/upstream/server-1/tools", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"echo"`) {
		t.Fatalf("tools = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"http://example.test/mcp","enabled":false,"expose":"none"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", recorder.Code, recorder.Body.String())
	}
	updated, ok := manager.Get("server-1")
	if !ok || updated.Name != "Updated" || updated.Enabled {
		t.Fatalf("updated = %#v", updated)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"http://example.test/mcp","enabled":false,"expose":"none","headers":{"Authorization":"<redacted>"},"env":{"API_TOKEN":"<redacted>"}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("redacted update = %d: %s", recorder.Code, recorder.Body.String())
	}
	updated, _ = manager.Get("server-1")
	if updated.Headers["Authorization"] != "Bearer secret" || updated.Env["API_TOKEN"] != "secret" {
		t.Fatalf("redacted values overwrote stored secrets: %#v %#v", updated.Headers, updated.Env)
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

func TestUpstreamAPIRejectsInvalidConfig(t *testing.T) {
	manager := upstream.NewManager(upstream.NewStore(filepath.Join(t.TempDir(), "upstream.json")))
	handler := New(API{Upstream: manager})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/upstream", strings.NewReader(`{"id":"server-1","name":"Server","transport":"http","enabled":true}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if len(manager.List()) != 0 {
		t.Fatalf("invalid server was persisted: %+v", manager.List())
	}
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
