package config

import (
	"context"
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
			cfg.Server.AllowUnauthenticatedLoopback = true
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

type persistAdmin struct {
	metadata tunnel.Metadata
}

func (a persistAdmin) FetchMetadata(context.Context, tunnel.Config) (tunnel.Metadata, error) {
	return a.metadata, nil
}
func (persistAdmin) ListManaged(context.Context, tunnel.Config, tunnel.AdminScope) ([]tunnel.Metadata, error) {
	return nil, nil
}
func (persistAdmin) GetManaged(context.Context, tunnel.Config, string) (tunnel.Metadata, error) {
	return tunnel.Metadata{}, nil
}
func (persistAdmin) CreateManaged(context.Context, tunnel.Config, tunnel.CreateRequest) (tunnel.Metadata, error) {
	return tunnel.Metadata{}, nil
}
func (persistAdmin) UpdateManaged(context.Context, tunnel.Config, string, tunnel.UpdateRequest) (tunnel.Metadata, error) {
	return tunnel.Metadata{}, nil
}
func (persistAdmin) DeleteManaged(context.Context, tunnel.Config, string) (tunnel.Metadata, error) {
	return tunnel.Metadata{}, nil
}
func (persistAdmin) VerifyAdminKey(context.Context, tunnel.Config) (tunnel.AdminAccess, int, error) {
	return tunnel.AdminAccess{}, 0, nil
}

func TestSyncTunnelMetadataCreatesMissingPersistedFile(t *testing.T) {
	tunnel.SetAdminBackend(persistAdmin{metadata: tunnel.Metadata{ID: "tunnel_test", Name: "Synced tunnel", Description: "Migrated cache", OrganizationIDs: []string{"org_test"}}})
	t.Cleanup(func() { tunnel.SetAdminBackend(nil) })
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := SaveAs(cfg, configformat.YAML); err != nil {
		t.Fatal(err)
	}

	metadata, path, err := SyncTunnelMetadata(context.Background(), tunnel.Config{ID: "tunnel_test", APIKey: "runtime-key"})
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

func TestTunnelMetadataRejectsSymlinkDirectoryEscape(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require Windows Developer Mode or elevation")
	}
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if err := SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, TunnelMetadataDir()); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_test", Name: "Outside"}); err == nil {
		t.Fatal("expected symlink tunnel metadata directory to be rejected")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("tunnel metadata escaped config root: %#v", entries)
	}
}
