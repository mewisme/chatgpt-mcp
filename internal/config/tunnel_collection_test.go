package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelCollectionSecretsAcrossFormats(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, "config"+configformat.Extension(format))
			secretPath := filepath.Join(root, "tunnel"+configformat.Extension(format))
			cfg := Default()
			instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_a", APIKey: "runtime-a"}, {ID: "tunnel_b", APIKey: "runtime-b", AdminProfileID: "default"}}
			admins := []tunnel.AdminConfig{{ID: "default", AdminKey: "admin-secret"}}
			cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
			if err := saveAt(configPath, secretPath, cfg); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "runtime-a") || strings.Contains(string(data), "runtime-b") || strings.Contains(string(data), "admin-secret") {
				t.Fatalf("config exposed keys: %s", data)
			}
			loaded, err := loadAt(configPath, secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := loaded.Tunnel.Collection().Instances[0].APIKey; got != "runtime-a" {
				t.Fatalf("runtime key = %q", got)
			}
			if got := loaded.Tunnel.Collection().Instances[1].APIKey; got != "runtime-b" {
				t.Fatalf("second runtime key = %q", got)
			}
			if got := loaded.Tunnel.Collection().Admins[0].AdminKey; got != "admin-secret" {
				t.Fatalf("admin key = %q", got)
			}
		})
	}
}

func TestLegacyScalarTunnelMigratesToCanonicalCollection(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.Enabled = true
	cfg.Tunnel.ID = "legacy"
	cfg.Tunnel.APIKey = "runtime-secret"
	cfg.Tunnel.AdminKey = "admin-secret"
	cfg.Tunnel.AdminOrganizationID = "org_legacy"
	cfg.Tunnel.ControlPlaneBaseURL = "https://example.test"
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAt(configPath, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	collection := loaded.Tunnel.Collection()
	if len(collection.Instances) != 1 || len(collection.Admins) != 1 {
		t.Fatalf("migrated collection = %#v", collection)
	}
	instance, admin := collection.Instances[0], collection.Admins[0]
	if instance.ID != "legacy" || instance.APIKey != "runtime-secret" || instance.AdminProfileID != "default" || instance.ControlPlaneBaseURL != "https://example.test" {
		t.Fatalf("migrated instance = %#v", instance)
	}
	if admin.ID != "default" || admin.AdminKey != "admin-secret" || admin.OrganizationID != "org_legacy" || admin.ControlPlaneBaseURL != "https://example.test" {
		t.Fatalf("migrated admin = %#v", admin)
	}
	if loaded.Tunnel.ID != "" || loaded.Tunnel.APIKey != "" || loaded.Tunnel.AdminKey != "" {
		t.Fatalf("legacy scalar fields survived migration: %#v", loaded.Tunnel)
	}
	persisted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(persisted)
	for _, secret := range []string{"runtime-secret", "admin-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("canonical config exposed %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, `"instances"`) || !strings.Contains(text, `"admins"`) || strings.Contains(text, `"api_key"`) || strings.Contains(text, `"admin_key"`) {
		t.Fatalf("legacy scalar tunnel fields survived canonical config: %s", text)
	}
	store := secretstore.New(root)
	if got, err := store.Get(instanceSecretName("legacy")); err != nil || got != "runtime-secret" {
		t.Fatalf("migrated runtime secret = %q err=%v", got, err)
	}
	if got, err := store.Get(adminProfileSecretName("default")); err != nil || got != "admin-secret" {
		t.Fatalf("migrated admin secret = %q err=%v", got, err)
	}
	if _, err := store.Get(tunnelRuntimeSecretName); !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("legacy runtime secret still stored: %v", err)
	}
	if _, err := store.Get(tunnelAdminSecretName); !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("legacy admin secret still stored: %v", err)
	}
}
