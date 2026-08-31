package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type adminUpstreamClient struct {
	tools    []upstream.Tool
	toolsErr error
}

func (*adminUpstreamClient) Connect(context.Context, upstream.Server) error { return nil }
func (*adminUpstreamClient) Close(context.Context, string) error            { return nil }
func (c *adminUpstreamClient) Tools(context.Context, string) ([]upstream.Tool, error) {
	if c.toolsErr != nil {
		return nil, c.toolsErr
	}
	return append([]upstream.Tool(nil), c.tools...), nil
}
func (*adminUpstreamClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return upstream.CallResult{}, nil
}
func (*adminUpstreamClient) PID(string) int { return 0 }

func TestTunnelConfigRedactsAPIKey(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnel = tunnel.Config{Enabled: true, ID: "tunnel_test", APIKey: "secret"}
	handler := New(API{Tunnel: tunnel.NewConfigured(cfg.Tunnel, nil), Config: config.NewRuntimeStore(cfg)})
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

func TestTunnelConfigureRollsBackRuntimeAndMemoryWhenPersistenceFails(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel = tunnel.Config{Enabled: false, ID: "tunnel_old", APIKey: "old-secret"}
	client := tunnel.NewConfigured(cfg.Tunnel, nil)
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Tunnel: client, Config: store, saveConfig: func(config.Config) error { return errors.New("persistence failed") }})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/tunnel", strings.NewReader(`{"enabled":false,"id":"tunnel_new","api_key":"new-secret"}`)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Tunnel; got.ID != "tunnel_old" || got.APIKey != "old-secret" {
		t.Fatalf("in-memory config changed after persistence failure: %#v", got)
	}
	if got := client.Config(); got.ID != "tunnel_old" || got.APIKey != "old-secret" {
		t.Fatalf("runtime config changed after persistence failure: %#v", got)
	}
}

func TestTunnelConfigurePreservesSecretFromSerializedConfigStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel = tunnel.Config{Enabled: false, ID: "tunnel_store", APIKey: "store-secret"}
	if err := config.SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	client := tunnel.NewConfigured(tunnel.Config{Enabled: false, ID: "tunnel_runtime", APIKey: "stale-runtime-secret"}, nil)
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Tunnel: client, Config: store})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/tunnel", strings.NewReader(`{"enabled":false,"id":"tunnel_new"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Tunnel; got.ID != "tunnel_new" || got.APIKey != "store-secret" {
		t.Fatalf("stored tunnel config = %#v", got)
	}
	if got := client.Config(); got.ID != "tunnel_new" || got.APIKey != "store-secret" {
		t.Fatalf("runtime tunnel config = %#v", got)
	}
}

func TestConfigAPIHidesTokenHashes(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret-hash"
	cfg.Auth.AdminTokenHash = "admin-secret-hash"
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(cfg)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
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

func TestConfigAPIWildcardExposureRequiresBothAuth(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store, saveConfig: func(config.Config) error { return nil }})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"server":{"port":37421,"expose":{"mode":"0.0.0.0","interfaces":[]}}}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "allow_insecure_http") {
		t.Fatalf("wildcard without HTTP opt-in status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"server":{"port":37421,"expose":{"mode":"0.0.0.0","interfaces":[]},"allow_insecure_http":true}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("wildcard status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"auth":{"mcp_enabled":false,"admin_enabled":true}}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("disable auth status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot(); got.Server.Expose.Mode != config.ExposureWildcard || !got.Auth.MCPEnabled || !got.Auth.AdminEnabled {
		t.Fatalf("invalid wildcard auth state committed: %#v", got)
	}
}

func TestHealthReportsAdminAuthState(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.AdminEnabled = false
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(cfg)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"auth_enabled":false`) {
		t.Fatalf("health = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestConfigAPIFeaturePatchUpdatesRuntimeCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithFeatures(cfg.Features)
	handler := New(API{Config: store, Tools: runtime})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"features":{"ponytail":{"enabled":false}}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Features; got.Ponytail.Enabled || !got.Caveman.Enabled {
		t.Fatalf("stored features = %#v", got)
	}
	if _, ok := runtime.Registry.Schema("ponytail_turn"); ok {
		t.Fatal("ponytail tool survived Admin disable")
	}
	if _, ok := runtime.Registry.Schema("caveman_turn"); !ok {
		t.Fatal("caveman tool disappeared after ponytail disable")
	}
	if !strings.Contains(recorder.Body.String(), `"features":{"ponytail":{"enabled":false},"caveman":{"enabled":true}}`) {
		t.Fatalf("feature config missing from response: %s", recorder.Body.String())
	}
}

func TestConfigAPIFeaturePersistenceFailureRollsBackRuntimeCatalog(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithFeatures(cfg.Features)
	handler := New(API{Config: store, Tools: runtime, saveConfig: func(config.Config) error { return errors.New("persistence failed") }})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"features":{"ponytail":{"enabled":false}}}`)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Features; !got.Ponytail.Enabled || !got.Caveman.Enabled {
		t.Fatalf("store changed after persistence failure: %#v", got)
	}
	if got := runtime.Features(); !got.Ponytail.Enabled || !got.Caveman.Enabled {
		t.Fatalf("runtime features changed after persistence failure: %#v", got)
	}
	if _, ok := runtime.Registry.Schema("ponytail_turn"); !ok {
		t.Fatal("ponytail tool was not restored after persistence failure")
	}
	if _, ok := runtime.Registry.Schema("caveman_turn"); !ok {
		t.Fatal("caveman tool disappeared after persistence failure")
	}
}

func TestConfigAPIFeatureRuntimeFailureRollsBackPersistedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Features.Caveman.Enabled = false
	if err := config.SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithFeatures(cfg.Features)
	if err := runtime.Registry.Register("caveman_turn", tools.Schema{Name: "caveman_turn"}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("collision"), nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: store, Tools: runtime})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"features":{"caveman":{"enabled":true}}}`)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Features; !got.Ponytail.Enabled || got.Caveman.Enabled {
		t.Fatalf("store changed after runtime sync failure: %#v", got)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Features.Ponytail.Enabled || loaded.Features.Caveman.Enabled {
		t.Fatalf("persisted config was not rolled back: %#v", loaded.Features)
	}
	if got := runtime.Features(); !got.Ponytail.Enabled || got.Caveman.Enabled {
		t.Fatalf("runtime features changed after failed sync: %#v", got)
	}
}

func TestConfigAPIPermissionsPatchUpdatesRuntimeAccess(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithAccess(cfg.Features, cfg.Permissions.AllowDirs)
	item, err := runtime.Workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.Workspaces.ResolveWorkingDirectory(item.ID, allowed); err == nil {
		t.Fatal("unconfigured directory was accessible")
	}
	handler := New(API{Config: store, Tools: runtime, saveConfig: func(config.Config) error { return nil }})
	body := fmt.Sprintf(`{"permissions":{"allow_dirs":[%q]}}`, allowed)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := runtime.Workspaces.ResolveWorkingDirectory(item.ID, allowed); err != nil {
		t.Fatalf("runtime access was not updated: %v", err)
	}
	canonicalAllowed, err := filepath.EvalSymlinks(allowed)
	if err != nil {
		t.Fatal(err)
	}
	canonicalAllowed = filepath.Clean(canonicalAllowed)
	if got := store.Snapshot().Permissions.AllowDirs; len(got) != 1 || got[0] != canonicalAllowed {
		t.Fatalf("stored permissions = %#v", got)
	}
	if !strings.Contains(recorder.Body.String(), `"permissions":{"allow_dirs":[`) {
		t.Fatalf("permissions missing from response: %s", recorder.Body.String())
	}
}

func TestConfigAPIPermissionsPersistenceFailureKeepsRuntimeAccess(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithAccess(cfg.Features, cfg.Permissions.AllowDirs)
	item, err := runtime.Workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: store, Tools: runtime, saveConfig: func(config.Config) error { return errors.New("persistence failed") }})
	body := fmt.Sprintf(`{"permissions":{"allow_dirs":[%q]}}`, allowed)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := runtime.Workspaces.ResolveWorkingDirectory(item.ID, allowed); err == nil {
		t.Fatal("runtime access changed after persistence failure")
	}
	if len(store.Snapshot().Permissions.AllowDirs) != 0 {
		t.Fatalf("store changed after persistence failure: %#v", store.Snapshot().Permissions)
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

func TestUpstreamAPIReportsProxyRefreshFailureAndPreservesCatalog(t *testing.T) {
	client := &adminUpstreamClient{tools: []upstream.Tool{{Name: "echo", InputSchema: map[string]any{"type": "object"}}}}
	manager := upstream.NewManagerWithClient(nil, client)
	server := upstream.Server{ID: "server-1", Name: "Server", Transport: "http", URL: "http://example.test/mcp", Enabled: true, Expose: "all"}
	if err := manager.Add(server); err != nil {
		t.Fatal(err)
	}
	runtime := &tools.Runtime{Registry: tools.NewRegistry(), Upstream: manager}
	if err := tools.RefreshUpstreamProxies(context.Background(), runtime.Registry, manager, false); err != nil {
		t.Fatal(err)
	}
	client.toolsErr = errors.New("upstream unavailable")
	handler := New(API{Upstream: manager, Tools: runtime})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"http://example.test/mcp","enabled":true,"expose":"all"}`)))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "proxy refresh failed") {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, ok := runtime.Registry.Schema("server-1__echo"); !ok {
		t.Fatal("previous proxy catalog was removed after Admin refresh failure")
	}
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
