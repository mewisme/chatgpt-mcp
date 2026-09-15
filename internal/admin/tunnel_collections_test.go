package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelCollectionAPIAttachesAndDetachesByID(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{{ID: "a", APIKey: "secret-a"}}
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	store := config.NewRuntimeStore(cfg)
	manager := tunnel.NewManager(&tools.Runtime{Registry: tools.NewRegistry()}, nil)
	if err := manager.Reconcile(context.Background(), cfg.RuntimeTunnels()); err != nil {
		t.Fatal(err)
	}
	api := API{Config: store, Tunnels: manager, saveConfig: func(config.Config) error { return nil }, ReloadConfig: func(next config.Config) error {
		if err := manager.Reconcile(context.Background(), next.RuntimeTunnels()); err != nil {
			return err
		}
		_, err := store.Update(func(config.Config) (config.Config, error) { return next, nil })
		return err
	}}
	handler := New(api)
	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/tunnels", bytes.NewBufferString(`{"id":"b","api_key":"secret-b"}`)))
	if post.Code != http.StatusOK || strings.Contains(post.Body.String(), "secret-b") {
		t.Fatalf("attach status=%d body=%s", post.Code, post.Body.String())
	}
	a, _ := manager.Client("a")
	if _, ok := manager.Client("b"); !ok {
		t.Fatal("b was not attached")
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
	if got, _ := manager.Client("a"); got != a {
		t.Fatal("detaching b replaced a")
	}
	if _, ok := manager.Client("b"); ok {
		t.Fatal("b remained attached")
	}
}

func TestAdminProfileMutationRequiresUnambiguousProfile(t *testing.T) {
	collection := tunnel.CollectionConfig{Admins: []tunnel.AdminConfig{{ID: "a"}, {ID: "b"}}}
	if _, err := resolveAdminProfile(collection, ""); err == nil {
		t.Fatal("ambiguous admin profile was accepted")
	}
	admin, err := resolveAdminProfile(collection, "b")
	if err != nil || admin.ID != "b" {
		t.Fatalf("profile=%+v err=%v", admin, err)
	}
}

func TestManagedTunnelAPIKeepsAllProfileProvenance(t *testing.T) {
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
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{{ID: "a", AdminKey: "key-a", OrganizationID: "org-a", ManageAccess: true, ControlPlaneBaseURL: server.URL}, {ID: "b", AdminKey: "key-b", OrganizationID: "org-b", ManageAccess: true, ControlPlaneBaseURL: server.URL}}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	manager := tunnel.NewManager(&tools.Runtime{Registry: tools.NewRegistry()}, nil)
	if err := manager.Reconcile(context.Background(), cfg.RuntimeTunnels()); err != nil {
		t.Fatal(err)
	}
	handler := New(API{Config: config.NewRuntimeStore(cfg), Tunnels: manager})
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
