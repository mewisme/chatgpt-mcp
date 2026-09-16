package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	securemcptunnel "go.mewis.me/chatgpt-mcp/plugins/secure-mcp-tunnel"
)

func TestTunnelCollectionAPIAttachesAndDetachesByID(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{{ID: "a", APIKey: "secret-a"}}
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewRuntimeStore(loaded)
	api := API{Config: store, saveConfig: func(config.Config) error { return nil }, ReloadConfig: func(next config.Config) error {
		_, err := store.Update(func(config.Config) (config.Config, error) { return next, nil })
		return err
	}}
	handler := New(api)
	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/tunnels", bytes.NewBufferString(`{"id":"b","api_key":"secret-b"}`)))
	if post.Code != http.StatusOK || strings.Contains(post.Body.String(), "secret-b") {
		t.Fatalf("attach status=%d body=%s", post.Code, post.Body.String())
	}
	if ids := store.Snapshot().RuntimeTunnels().Instances; len(ids) != 2 || ids[0].ID != "a" || ids[1].ID != "b" {
		t.Fatalf("instances after attach = %#v", ids)
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/tunnels", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "secret-a") || strings.Contains(list.Body.String(), "secret-b") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var views []localTunnelView
	if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || !views[0].RuntimeKeyConfigured || !views[1].RuntimeKeyConfigured {
		t.Fatalf("views=%+v", views)
	}
	detach := httptest.NewRecorder()
	handler.ServeHTTP(detach, httptest.NewRequest(http.MethodDelete, "/api/tunnels/b", nil))
	if detach.Code != http.StatusNoContent {
		t.Fatalf("detach status=%d body=%s", detach.Code, detach.Body.String())
	}
	if ids := store.Snapshot().RuntimeTunnels().Instances; len(ids) != 1 || ids[0].ID != "a" {
		t.Fatalf("instances after detach = %#v", ids)
	}
}

func TestTunnelAdminCollectionCRUDPreservesSecret(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	tunnel.SetAdminBackend(securemcptunnel.ControlPlane())
	t.Cleanup(func() { tunnel.SetAdminBackend(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels" {
			_, _ = w.Write([]byte(`{"tunnels":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewRuntimeStore(loaded)
	api := API{Config: store, saveConfig: func(config.Config) error { return nil }, ReloadConfig: func(next config.Config) error {
		_, err := store.Update(func(config.Config) (config.Config, error) { return next, nil })
		return err
	}}
	handler := New(api)
	body := fmt.Sprintf(`{"id":"work","admin_key":"admin-secret","organization_id":"org_one","control_plane_base_url":%q}`, server.URL)
	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/tunnel-admins", bytes.NewBufferString(body)))
	if post.Code != http.StatusOK || strings.Contains(post.Body.String(), "admin-secret") || !strings.Contains(post.Body.String(), `"key_configured":true`) {
		t.Fatalf("post status=%d body=%s", post.Code, post.Body.String())
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/tunnel-admins/work", nil))
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "admin-secret") || !strings.Contains(get.Body.String(), `"organization_id":"org_one"`) {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/api/tunnel-admins/work", bytes.NewBufferString(fmt.Sprintf(`{"organization_id":"org_two","control_plane_base_url":%q}`, server.URL))))
	if put.Code != http.StatusOK || strings.Contains(put.Body.String(), "admin-secret") || !strings.Contains(put.Body.String(), `"organization_id":"org_two"`) {
		t.Fatalf("put status=%d body=%s", put.Code, put.Body.String())
	}
	collection := store.Snapshot().RuntimeTunnels()
	if len(collection.Admins) != 1 || collection.Admins[0].AdminKey != "admin-secret" || collection.Admins[0].OrganizationID != "org_two" {
		t.Fatalf("admins=%#v", collection.Admins)
	}
	if _, _, err := application.UpdateTunnelAdminProfile(t.Context(), tunnel.AdminConfig{ID: "work", OrganizationID: "org_cli"}); err != nil {
		t.Fatal(err)
	}
	if err := api.reloadCollectionFromDisk(); err != nil {
		t.Fatal(err)
	}
	if got := store.Snapshot().RuntimeTunnels().Admins; len(got) != 1 || got[0].AdminKey != "admin-secret" || got[0].OrganizationID != "org_cli" {
		t.Fatalf("application update admins=%#v", got)
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/tunnel-admins", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"work"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	remove := httptest.NewRecorder()
	handler.ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/tunnel-admins/work", nil))
	if remove.Code != http.StatusNoContent || len(store.Snapshot().RuntimeTunnels().Admins) != 0 {
		t.Fatalf("delete status=%d body=%s admins=%#v", remove.Code, remove.Body.String(), store.Snapshot().RuntimeTunnels().Admins)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/tunnel-admins/work", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestManagedTunnelAPIKeepsAllProfileProvenance(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	tunnel.SetAdminBackend(securemcptunnel.ControlPlane())
	t.Cleanup(func() { tunnel.SetAdminBackend(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tunnels" {
			http.NotFound(w, r)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer key-a":
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"shared"},{"id":"a-only"}]}`))
		case "Bearer key-b":
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"shared"},{"id":"b-only"}]}`))
		default:
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{{ID: "a", AdminKey: "key-a", OrganizationID: "org-a", ManageAccess: true, ControlPlaneBaseURL: server.URL}, {ID: "b", AdminKey: "key-b", OrganizationID: "org-b", ManageAccess: true, ControlPlaneBaseURL: server.URL}}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: config.NewRuntimeStore(loaded)})
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/managed-tunnels", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", list.Code, list.Body.String())
	}
	var views []managedTunnelView
	if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 3 || views[0].Metadata.ID != "shared" || len(views[0].AdminProfiles) != 2 {
		t.Fatalf("views=%+v", views)
	}
	ambiguous := httptest.NewRecorder()
	handler.ServeHTTP(ambiguous, httptest.NewRequest(http.MethodDelete, "/api/managed-tunnels/shared", nil))
	if ambiguous.Code != http.StatusBadRequest {
		t.Fatalf("ambiguous delete status=%d body=%s", ambiguous.Code, ambiguous.Body.String())
	}
}

func TestTunnelAdminAPIRejectsInvalidProfileID(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Admins = &admins
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewRuntimeStore(loaded)
	api := API{Config: store, saveConfig: func(config.Config) error { return nil }, ReloadConfig: func(next config.Config) error {
		_, err := store.Update(func(config.Config) (config.Config, error) { return next, nil })
		return err
	}}
	post := httptest.NewRecorder()
	New(api).ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/tunnel-admins", bytes.NewBufferString(`{"id":"my profile","admin_key":"admin-secret","organization_id":"org_one"}`)))
	if post.Code != http.StatusBadRequest || !strings.Contains(post.Body.String(), "letters, numbers") {
		t.Fatalf("status=%d body=%s", post.Code, post.Body.String())
	}
}
