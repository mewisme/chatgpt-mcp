package cli

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

func TestSetConfigValueTyped(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	if err := setConfigValue(&cfg, "server.port", "4000"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "server.expose", "true"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "admin.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "tunnel.control_plane_base_url", "https://api.openai.com"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "tunnel.organization_id", "org-test"); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4000 || !cfg.Server.Expose || cfg.Admin.Enabled || cfg.Tunnel.ControlPlaneBaseURL != "https://api.openai.com" || cfg.Tunnel.OrganizationID != "org-test" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestSensitiveConfigValuesCannotBeRead(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "secret"
	cfg.Tunnel.APIKey = "secret"
	for _, key := range []string{"auth.mcp_token_hash", "tunnel.api_key"} {
		if _, err := getConfigValue(cfg, key); err == nil {
			t.Fatalf("expected %s to be rejected", key)
		}
	}
}

func TestConfigPresetApplyPreservesSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "tunnel-secret"
	cfg.Tunnel.ID = "tunnel-id"
	cfg.Tunnel.ControlPlaneBaseURL = "https://api.openai.com"
	cfg.Tunnel.OrganizationID = "org-test"
	if err := config.ApplyPreset(&cfg, "lan"); err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.Expose || cfg.Admin.Enabled {
		t.Fatalf("preset not applied: %#v", cfg)
	}
	if cfg.Auth.MCPTokenHash != "mcp-secret" || cfg.Auth.AdminTokenHash != "admin-secret" {
		t.Fatal("auth secrets changed")
	}
	if cfg.Tunnel.APIKey != "tunnel-secret" || cfg.Tunnel.ID != "tunnel-id" ||
		cfg.Tunnel.ControlPlaneBaseURL != "https://api.openai.com" || cfg.Tunnel.OrganizationID != "org-test" {
		t.Fatal("tunnel details changed")
	}
}

func TestConfigPresetRequiresConfiguredAuth(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = ""
	cfg.Auth.AdminTokenHash = ""
	if err := config.ApplyPreset(&cfg, "default"); err == nil {
		t.Fatal("preset unexpectedly bypassed auth validation")
	}
}

func TestMatchingConfigPreset(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp"
	cfg.Auth.AdminTokenHash = "admin"
	if got := config.MatchPreset(cfg); got != "default" {
		t.Fatalf("preset = %q", got)
	}
	cfg.Server.Port++
	if got := config.MatchPreset(cfg); got != "custom" {
		t.Fatalf("preset = %q", got)
	}
}

func TestUnknownConfigPreset(t *testing.T) {
	if _, err := config.PresetByName("missing"); err == nil {
		t.Fatal("unknown preset was accepted")
	}
}

func TestRemoveConfigRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "chatgpt-mcp")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeConfigRoot(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root still exists: %v", err)
	}
	if err := removeConfigRoot(t.TempDir()); err == nil {
		t.Fatal("unsafe root was not rejected")
	}
}
