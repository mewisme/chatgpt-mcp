package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelMetadataRoundTripAcrossFormats(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			defer configformat.SetRootPath("")
			root := filepath.Join(t.TempDir(), "config")
			if err := configformat.SetRootPath(root); err != nil {
				t.Fatal(err)
			}
			cfg := Default()
			cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
			if err := SaveAs(cfg, format); err != nil {
				t.Fatal(err)
			}
			metadata := tunnel.Metadata{ID: "tunnel_test", Name: "Test tunnel", Description: "Persisted", WorkspaceIDs: []string{"ws_test"}, FetchedAt: time.Now().UTC().Truncate(time.Second)}
			path, err := SaveTunnelMetadata(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(path) != configformat.Extension(format) {
				t.Fatalf("path = %s", path)
			}
			loaded, err := LoadTunnelMetadata(metadata.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.ID != metadata.ID || loaded.Name != metadata.Name || len(loaded.WorkspaceIDs) != 1 || loaded.WorkspaceIDs[0] != "ws_test" {
				t.Fatalf("metadata = %#v", loaded)
			}
			if err := RemoveTunnelMetadata(metadata.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("metadata file survived removal: %v", err)
			}
		})
	}
}

func TestSyncTunnelMetadataCreatesMissingPersistedFile(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	if err := SaveAs(cfg, configformat.YAML); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tunnels/tunnel_test" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer runtime-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"id":"tunnel_test","name":"Synced tunnel","description":"Migrated cache","organization_ids":["org_test"]}`))
	}))
	defer server.Close()

	metadata, path, err := SyncTunnelMetadata(context.Background(), tunnel.Config{ID: "tunnel_test", APIKey: "runtime-key", ControlPlaneBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "Synced tunnel" || filepath.Ext(path) != ".yaml" {
		t.Fatalf("metadata=%#v path=%s", metadata, path)
	}
	loaded, err := LoadTunnelMetadata("tunnel_test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != metadata.Name || len(loaded.OrganizationIDs) != 1 || loaded.OrganizationIDs[0] != "org_test" {
		t.Fatalf("persisted metadata = %#v", loaded)
	}
}

func TestTunnelMetadataPathRejectsTraversal(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "nested/id", `nested\\id`} {
		if _, err := TunnelMetadataPath(id); err == nil {
			t.Fatalf("accepted unsafe id %q", id)
		}
	}
}
