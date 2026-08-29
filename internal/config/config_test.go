package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTunnelOrigin(t *testing.T) {
	cfg := Default()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 4321
	if got := TunnelOrigin(cfg); got != "http://127.0.0.1:4321" {
		t.Fatalf("unexpected origin: %s", got)
	}
}

func TestValidateRequiresAuthTokens(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing auth token validation error")
	}
	cfg.Auth.MCPTokenHash = "configured"
	cfg.Auth.AdminTokenHash = "configured"
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTunnelCommand(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.Enabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected tunnel command validation error")
	}
}

func TestConfigSaveSeparatesTunnelAPIKey(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.APIKey = "tunnel-secret"

	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "tunnel-secret") || strings.Contains(string(configData), `"api_key"`) {
		t.Fatalf("config.json leaked tunnel secret: %s", configData)
	}
	secretData, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secretData), "tunnel-secret") {
		t.Fatalf("tunnel.json did not contain secret: %s", secretData)
	}
	loaded, err := loadAt(configPath, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tunnel.APIKey != "tunnel-secret" {
		t.Fatalf("loaded API key = %q", loaded.Tunnel.APIKey)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(secretPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("tunnel secret mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestLegacyTunnelAPIKeyMigratesOnSave(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.APIKey = "legacy-secret"
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
	if loaded.Tunnel.APIKey != "legacy-secret" {
		t.Fatalf("legacy API key = %q", loaded.Tunnel.APIKey)
	}
	if err := saveAt(configPath, secretPath, loaded); err != nil {
		t.Fatal(err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "legacy-secret") || strings.Contains(string(configData), `"api_key"`) {
		t.Fatalf("legacy secret was not migrated out of config.json: %s", configData)
	}
	secret, err := loadTunnelSecretAt(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "legacy-secret" {
		t.Fatalf("migrated secret = %q", secret)
	}
}

func TestClearingTunnelAPIKeyRemovesSecretFile(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.APIKey = "secret"
	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Tunnel.APIKey = ""
	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
		t.Fatalf("secret file still exists: %v", err)
	}
}
