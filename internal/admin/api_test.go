package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tools"
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

func TestSingletonTunnelRoutesAreRemoved(t *testing.T) {
	handler := New(API{Config: config.NewRuntimeStore(config.Default())})
	for _, path := range []string{"/api/tunnel", "/api/tunnel/config", "/api/tunnel/admin/key", "/api/tunnel/managed"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestConfigAPIHidesTokenHashes(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret-hash"
	cfg.Auth.AdminTokenHash = "admin-secret-hash"
	recorder := httptest.NewRecorder()
	New(API{Config: config.NewRuntimeStore(cfg)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.Contains(body, "secret-hash") || strings.Contains(body, "token_hash") || strings.Contains(body, `"token":`) {
		t.Fatalf("config API leaked token hashes: %s", body)
	}
	if !strings.Contains(body, `"mcp_token_configured":true`) || !strings.Contains(body, `"admin_token_configured":true`) || !strings.Contains(body, `"mcp_token_revealable":false`) {
		t.Fatalf("configured state missing: %s", body)
	}
	if strings.Contains(body, `"host"`) || !strings.Contains(body, `"server":{"enabled":true`) || !strings.Contains(body, `"expose":{"mode":"none","interfaces":[]}`) {
		t.Fatalf("server exposure view is invalid: %s", body)
	}
}

func TestConfigAPIMutationAutomaticallyReloadsRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	store := config.NewRuntimeStore(cfg)
	saved, reloaded := false, false
	handler := New(API{
		Config: store,
		saveConfig: func(next config.Config) error {
			saved = true
			if next.Server.Port != 41021 {
				t.Fatalf("saved port=%d", next.Server.Port)
			}
			return nil
		},
		ReloadConfig: func(next config.Config) error {
			reloaded = true
			if next.Server.Port != 41021 {
				t.Fatalf("reloaded port=%d", next.Server.Port)
			}
			return nil
		},
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"server":{"port":41021}}`)))
	if recorder.Code != http.StatusOK || !saved || !reloaded {
		t.Fatalf("status=%d saved=%t reloaded=%t body=%s", recorder.Code, saved, reloaded, recorder.Body.String())
	}
}

func TestConfigAPIRejectsDisablingLastMCPTransport(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store, saveConfig: func(config.Config) error { t.Fatal("invalid transport config must not persist"); return nil }})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"server":{"enabled":false}}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "at least one MCP transport") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !store.Snapshot().Server.Enabled {
		t.Fatal("invalid transport config mutated store")
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

func TestConfigAPIPartialAuthPatchPreservesOmittedSetting(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store, saveConfig: func(config.Config) error { return nil }})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"auth":{"mcp_enabled":true}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	got := store.Snapshot()
	if !got.Auth.MCPEnabled || !got.Auth.AdminEnabled {
		t.Fatalf("partial auth patch changed omitted setting: %#v", got.Auth)
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

func TestConfigAPIOmitsLegacyInteractiveField(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store, saveConfig: func(config.Config) error { return nil }})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"interactive"`) {
		t.Fatalf("legacy interactive field exposed: %s", recorder.Body.String())
	}
}

func TestConfigAPIIgnoresLegacyFeaturesPatch(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"features":{"ponytail":{"active":false},"caveman":{"mode":"review"}}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"features"`) {
		t.Fatalf("features leaked into public config: %s", recorder.Body.String())
	}
}

func TestConfigAPIPermissionsPatchUpdatesRuntimeAccess(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	root := t.TempDir()
	allowed := t.TempDir()
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithAccess(cfg.Permissions.AllowDirs)
	item, err := runtime.Workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.Workspaces.ResolveDirectory(item.ID, allowed); err == nil {
		t.Fatal("unconfigured directory was accessible")
	}
	handler := New(API{Config: store, Tools: runtime, saveConfig: func(config.Config) error { return nil }})
	body := fmt.Sprintf(`{"permissions":{"allow_dirs":[%q]}}`, allowed)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := runtime.Workspaces.ResolveDirectory(item.ID, allowed); err != nil {
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

func TestConfigAPIShellPathPatch(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	handler := New(API{Config: store, saveConfig: func(config.Config) error { return nil }})
	path := filepath.Join(t.TempDir(), "bin")
	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{"shell":{"path":[%q]}}`, path)
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := store.Snapshot().Shell.Path; len(got) != 1 || got[0] != filepath.Clean(path) {
		t.Fatalf("shell path = %#v", got)
	}
	if !strings.Contains(recorder.Body.String(), `"shell":{"path":[`) {
		t.Fatalf("shell missing from response: %s", recorder.Body.String())
	}
}

func TestConfigAPIPermissionsPersistenceFailureKeepsRuntimeAccess(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	root := t.TempDir()
	allowed := t.TempDir()
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	store := config.NewRuntimeStore(cfg)
	runtime := tools.NewRuntimeWithAccess(cfg.Permissions.AllowDirs)
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
	if _, _, err := runtime.Workspaces.ResolveDirectory(item.ID, allowed); err == nil {
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

func TestWorkspaceAPIRelocate(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	oldRoot := t.TempDir()
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	handler := New(API{Workspaces: manager})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/workspaces/"+item.ID+"/relocate", strings.NewReader(`{"path":`+jsonString(newRoot)+`}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("relocate status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var relocated workspace.Workspace
	if err := json.Unmarshal(recorder.Body.Bytes(), &relocated); err != nil {
		t.Fatal(err)
	}
	if relocated.ID != item.ID || relocated.Path == item.Path {
		t.Fatalf("relocated=%#v", relocated)
	}
	resolved, err := manager.Get(item.ID)
	if err != nil || resolved.ID != item.ID || resolved.Path != relocated.Path {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}

func TestWorkspaceAPIDeleteState(t *testing.T) {
	manager := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	root := t.TempDir()
	item, err := manager.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Workspaces: manager})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+item.ID+"/state", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete-state status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := manager.Get(item.ID); err == nil {
		t.Fatal("workspace still registered")
	}
	if _, err := os.Stat(filepath.Join(root, ".cgm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local state remains: %v", err)
	}
}

func TestUpstreamAPIManagementAndRedaction(t *testing.T) {
	client := &adminUpstreamClient{tools: []upstream.Tool{{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object"}}}}
	manager := upstream.NewManagerWithClient(nil, client)
	handler := New(API{Upstream: manager})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/upstream", strings.NewReader(`{"id":"server-1","name":"Server","transport":"http","url":"https://example.test/mcp","enabled":true,"headers":{"Authorization":"Bearer secret","X-Test":"ok"},"env":{"API_TOKEN":"secret","MODE":"test"}}`)))
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
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"https://example.test/mcp","enabled":false,"expose":"none"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", recorder.Code, recorder.Body.String())
	}
	updated, ok := manager.Get("server-1")
	if !ok || updated.Name != "Updated" || updated.Enabled {
		t.Fatalf("updated = %#v", updated)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"https://example.test/mcp","enabled":false,"expose":"none","headers":{"Authorization":"<redacted>"},"env":{"API_TOKEN":"<redacted>"}}`)))
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
	server := upstream.Server{ID: "server-1", Name: "Server", Transport: "http", URL: "https://example.test/mcp", Enabled: true, Expose: "all"}
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
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/upstream/server-1", strings.NewReader(`{"name":"Updated","transport":"http","url":"https://example.test/mcp","enabled":true,"expose":"all"}`)))
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
