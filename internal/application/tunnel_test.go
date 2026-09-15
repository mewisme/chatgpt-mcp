package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestAttachManagedTunnelSyncsMetadataAndPersistsSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tunnels/tunnel_runtime" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer admin-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"id":"tunnel_runtime","name":"Runtime","description":"Runtime tunnel","organization_ids":["org_runtime"]}`))
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, nil, []tunnel.AdminConfig{{ID: "default", AdminKey: "admin-secret", OrganizationID: "org_runtime", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}})

	item, err := AttachManagedTunnel(t.Context(), "tunnel_runtime", "default", "runtime-secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "tunnel_runtime" || !item.Enabled || !item.RuntimeKeyConfigured || item.AdminProfileID != "default" || item.OrganizationID != "org_runtime" || item.Status.Metadata == nil || item.Status.Metadata.Name != "Runtime" {
		t.Fatalf("item=%#v", item)
	}
	metadata, err := config.LoadTunnelMetadata(item.ID)
	if err != nil || metadata.Name != "Runtime" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 1 || instances[0].ID != item.ID || instances[0].APIKey != "runtime-secret" || instances[0].AdminProfileID != "default" {
		t.Fatalf("instances=%#v", instances)
	}
	assertTunnelSecretNotInManagedFiles(t, "runtime-secret")
}

func TestTunnelAdminProfileAndManagedLifecycle(t *testing.T) {
	created, updated, deleted := false, false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels":
			if r.URL.Query().Get("workspace_id") != "ws_admin" {
				t.Fatalf("scope query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"tunnel_one","name":"One","description":"First","workspace_ids":["ws_admin"]}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels/tunnel_one":
			_, _ = w.Write([]byte(`{"id":"tunnel_one","name":"One","description":"First","workspace_ids":["ws_admin"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tunnels":
			created = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["name"] != "Created" || body["description"] != "Created tunnel" {
				t.Fatalf("create body=%#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"tunnel_created","name":"Created","description":"Created tunnel","workspace_ids":["ws_admin"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tunnels/tunnel_created":
			updated = true
			_, _ = w.Write([]byte(`{"id":"tunnel_created","name":"Renamed","description":"Created tunnel","workspace_ids":["ws_admin"]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/tunnels/tunnel_created":
			deleted = true
			_, _ = w.Write([]byte(`{"id":"tunnel_created","name":"Renamed","description":"Created tunnel","workspace_ids":["ws_admin"]}`))
		default:
			t.Fatalf("unexpected request=%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	profile, err := AddTunnelAdminProfile(t.Context(), tunnel.AdminConfig{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ControlPlaneBaseURL: server.URL})
	if err != nil || !profile.KeyConfigured {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
	verified, count, err := VerifyTunnelAdminProfile(t.Context(), "work")
	if err != nil || count != 1 || !verified.ReadAccess || !verified.ManageAccess {
		t.Fatalf("verified=%#v count=%d err=%v", verified, count, err)
	}
	assertTunnelSecretNotInManagedFiles(t, "admin-secret")

	items, err := DiscoverManagedTunnels(t.Context(), "work")
	if err != nil || len(items) != 1 || items[0].Metadata.ID != "tunnel_one" || len(items[0].AdminProfiles) != 1 || items[0].AdminProfiles[0] != "work" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	got, err := GetManagedTunnelByProfile(t.Context(), "tunnel_one", "work")
	if err != nil || got.Metadata.Name != "One" {
		t.Fatalf("get=%#v err=%v", got, err)
	}
	createdResult, err := CreateManagedTunnelByProfile(t.Context(), "work", tunnel.CreateRequest{Name: "Created", Description: "Created tunnel", WorkspaceIDs: []string{"ws_admin"}})
	if err != nil || !created || createdResult.Metadata.ID != "tunnel_created" {
		t.Fatalf("create=%#v called=%t err=%v", createdResult, created, err)
	}
	name := "Renamed"
	updatedResult, err := UpdateManagedTunnelByProfile(t.Context(), "tunnel_created", "work", tunnel.UpdateRequest{Name: &name})
	if err != nil || !updated || updatedResult.Metadata.Name != "Renamed" {
		t.Fatalf("update=%#v called=%t err=%v", updatedResult, updated, err)
	}
	result, err := DeleteManagedTunnelByProfile(t.Context(), "tunnel_created", "work")
	if err != nil || !deleted || result.ID != "tunnel_created" {
		t.Fatalf("delete=%#v called=%t err=%v", result, deleted, err)
	}
	if _, err := config.LoadTunnelMetadata("tunnel_created"); !os.IsNotExist(err) {
		t.Fatalf("deleted metadata remains: %v", err)
	}
	if err := RemoveTunnelAdminProfile(t.Context(), "work"); err != nil {
		t.Fatal(err)
	}
	profiles, err := TunnelAdminProfiles()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
}

func TestAttachManagedTunnelAutoGeneratesRuntimeKey(t *testing.T) {
	const tunnelID = "tunnel_00000000000000000000000000000003"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels/"+tunnelID:
			_, _ = w.Write([]byte(`{"id":"` + tunnelID + `","name":"Auto","description":"Auto","organization_ids":["org_admin"]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organization/projects":
			_, _ = w.Write([]byte(`{"data":[{"id":"proj_default","name":"Default project","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organization/projects/proj_default/service_accounts":
			_, _ = w.Write([]byte(`{"data":[{"id":"svc_runtime","name":"chatgpt-mcp tunnel runtime"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organization/projects/proj_default/service_accounts/svc_runtime/api_keys":
			var body struct {
				Scopes []string `json:"scopes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Scopes) != 2 || body.Scopes[0] != "api.organization.tunnel.read" || body.Scopes[1] != "api.organization.tunnel.use" {
				t.Fatalf("scopes=%#v", body.Scopes)
			}
			_, _ = w.Write([]byte(`{"id":"key_runtime","value":"sk-runtime-generated"}`))
		default:
			t.Fatalf("unexpected request=%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, nil, []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_admin", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}})

	result, err := AttachManagedTunnelWithOptions(t.Context(), tunnelID, AttachManagedTunnelOptions{AdminProfileID: "work", AutoGenerateRuntimeKey: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != tunnelID || !result.Enabled || !result.RuntimeKeyConfigured || result.AdminProfileID != "work" || result.OrganizationID != "org_admin" {
		t.Fatalf("result=%#v", result)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 1 || instances[0].ID != tunnelID || instances[0].APIKey != "sk-runtime-generated" || instances[0].AdminProfileID != "work" {
		t.Fatalf("instances=%#v", instances)
	}
	assertTunnelSecretNotInManagedFiles(t, "sk-runtime-generated")
}

func TestDeleteManagedTunnelRequiresLocalDetach(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/tunnels/tunnel_selected" {
			deleted = true
			_, _ = w.Write([]byte(`{"id":"tunnel_selected","name":"Selected","description":"Selected","organization_ids":["org_admin"]}`))
			return
		}
		t.Fatalf("unexpected request=%s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_selected", APIKey: "runtime-secret", AdminProfileID: "work", OrganizationID: "org_admin"}}, []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_admin", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}})
	if _, err := DeleteManagedTunnelByProfile(t.Context(), "tunnel_selected", "work"); err == nil || !strings.Contains(err.Error(), "detach local tunnel") {
		t.Fatalf("attached remote deletion err=%v", err)
	}
	if deleted {
		t.Fatal("remote tunnel was deleted before local detach")
	}
	if err := DetachLocalTunnel(t.Context(), "tunnel_selected"); err != nil {
		t.Fatal(err)
	}
	result, err := DeleteManagedTunnelByProfile(t.Context(), "tunnel_selected", "work")
	if err != nil || !deleted || result.ID != "tunnel_selected" {
		t.Fatalf("result=%#v deleted=%t err=%v", result, deleted, err)
	}
}

func TestTunnelOnlyConfigCannotDisableTunnel(t *testing.T) {
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_only", APIKey: "runtime-secret"}}, nil)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Enabled = false
	cfg.Admin.Enabled = false
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := SetLocalTunnelEnabled(t.Context(), "tunnel_only", false); err == nil || !strings.Contains(err.Error(), "at least one MCP transport") {
		t.Fatalf("disable sole tunnel err=%v", err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 1 || !instances[0].Enabled || loaded.Server.Enabled {
		t.Fatalf("invalid transport mutation persisted: server=%#v instances=%#v", loaded.Server, instances)
	}
}

func TestUpdateLocalTunnelPreservesRuntimeKeyAndMutatesOnlyTarget(t *testing.T) {
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, []tunnel.InstanceConfig{
		{Enabled: true, ID: "tunnel_one", APIKey: "runtime-one", AdminProfileID: "work", OrganizationID: "org_old"},
		{Enabled: true, ID: "tunnel_two", APIKey: "runtime-two"},
	}, []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", ReadAccess: true, ManageAccess: true}})

	updated, err := UpdateLocalTunnel(t.Context(), tunnel.InstanceConfig{Enabled: false, ID: "tunnel_one", AdminProfileID: "work", OrganizationID: "org_new"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "tunnel_one" || updated.Enabled || !updated.RuntimeKeyConfigured || updated.OrganizationID != "org_new" {
		t.Fatalf("updated=%#v", updated)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 2 || instances[0].APIKey != "runtime-one" || instances[0].OrganizationID != "org_new" || instances[1].APIKey != "runtime-two" || !instances[1].Enabled {
		t.Fatalf("instances=%#v", instances)
	}
	if _, err := UpdateLocalTunnel(t.Context(), tunnel.InstanceConfig{ID: "missing"}); err == nil || !strings.Contains(err.Error(), "not attached") {
		t.Fatalf("missing update err=%v", err)
	}
}

func TestUpdateTunnelAdminProfilePreservesKeyAndAccess(t *testing.T) {
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, nil, []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_old", WorkspaceID: "ws_old", ReadAccess: true, ManageAccess: true}})

	updated, err := UpdateTunnelAdminProfile(t.Context(), tunnel.AdminConfig{ID: "work", OrganizationID: "org_new", WorkspaceID: "ws_new"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "work" || !updated.KeyConfigured || !updated.ReadAccess || !updated.ManageAccess || updated.OrganizationID != "org_new" || updated.WorkspaceID != "ws_new" {
		t.Fatalf("updated=%#v", updated)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	admins := loaded.RuntimeTunnels().Admins
	if len(admins) != 1 || admins[0].AdminKey != "admin-secret" || !admins[0].ReadAccess || !admins[0].ManageAccess || admins[0].OrganizationID != "org_new" {
		t.Fatalf("admins=%#v", admins)
	}
	if _, err := UpdateTunnelAdminProfile(t.Context(), tunnel.AdminConfig{ID: "missing"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing update err=%v", err)
	}
}

func TestManagedCreateFailureDoesNotChangeRuntimeCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	saveTunnelCollectionFixture(t, []tunnel.InstanceConfig{{ID: "tunnel_existing", APIKey: "runtime-secret"}}, []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ReadAccess: true, ManageAccess: true, ControlPlaneBaseURL: server.URL}})
	_, err := CreateManagedTunnelByProfile(context.Background(), "work", tunnel.CreateRequest{Name: "Broken", Description: "Broken", WorkspaceIDs: []string{"ws_admin"}})
	if err == nil {
		t.Fatal("create unexpectedly succeeded")
	}
	loaded, loadErr := config.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 1 || instances[0].ID != "tunnel_existing" || instances[0].APIKey != "runtime-secret" || instances[0].Enabled {
		t.Fatalf("failed remote mutation changed local runtime collection: %#v", instances)
	}
}

func setupTunnelApplicationRoot(t *testing.T, tunnelConfig tunnel.Config) {
	t.Helper()
	deferRoot := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(deferRoot) })
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	cfg.Tunnel = tunnelConfig
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func saveTunnelCollectionFixture(t *testing.T, instances []tunnel.InstanceConfig, admins []tunnel.AdminConfig) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instanceCopy := append([]tunnel.InstanceConfig(nil), instances...)
	adminCopy := append([]tunnel.AdminConfig(nil), admins...)
	cfg.Tunnel = tunnel.Config{Instances: &instanceCopy, Admins: &adminCopy}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func assertTunnelSecretNotInManagedFiles(t *testing.T, secret string) {
	t.Helper()
	for _, path := range []string{config.Path(), configformat.StructuredPath(config.RootPath(), "tunnel")} {
		data, err := os.ReadFile(path)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret leaked in %s: %s", path, data)
		}
	}
}
