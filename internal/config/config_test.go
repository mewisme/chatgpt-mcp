package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

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

func TestValidateBuiltinOpenAITunnel(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.Enabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing tunnel id/api key validation error")
	}
	cfg.Tunnel.ID = "tunnel_0123456789abcdef0123456789abcdef"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing tunnel API key validation error")
	}
	cfg.Tunnel.APIKey = "sk-test"
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Tunnel.ControlPlaneBaseURL = "not-a-url"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid control plane URL")
	}
}

func TestConfigSaveSeparatesTunnelAPIKey(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.ID = "tunnel_0123456789abcdef0123456789abcdef"
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
	if loaded.Tunnel.APIKey != "tunnel-secret" || loaded.Tunnel.ID != cfg.Tunnel.ID {
		t.Fatalf("loaded tunnel = %#v", loaded.Tunnel)
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

func TestConfigRoundTripAcrossFormats(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			configPath := configformat.PathFor(root, "config", format)
			secretPath := configformat.PathFor(root, "tunnel", format)
			cfg := Default()
			cfg.Auth.MCPTokenHash = "mcp-hash"
			cfg.Auth.AdminTokenHash = "admin-hash"
			cfg.Tunnel.ID = "tunnel_0123456789abcdef0123456789abcdef"
			cfg.Tunnel.APIKey = "tunnel-secret"
			if err := saveAt(configPath, secretPath, cfg); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadAt(configPath, secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Server.Port != cfg.Server.Port || loaded.Auth.MCPTokenHash != cfg.Auth.MCPTokenHash || loaded.Tunnel.APIKey != cfg.Tunnel.APIKey {
				t.Fatalf("round trip = %#v", loaded)
			}
			mainData, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(mainData), "tunnel-secret") {
				t.Fatalf("main %s config leaked tunnel secret: %s", format, mainData)
			}
		})
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

func TestLegacyGenericTunnelFieldsAreIgnored(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	data := []byte(`{
		"server":{"host":"127.0.0.1","port":37421},
		"admin":{"enabled":true,"port":37422},
		"auth":{"mcp_enabled":false,"admin_enabled":false},
		"tunnel":{"enabled":false,"id":"tunnel_test","command":"cloudflared","args":["tunnel"],"origin":"http://127.0.0.1:37421","public_url":"https://old.example"}
	}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAt(configPath, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tunnel.ID != "tunnel_test" {
		t.Fatalf("tunnel id = %q", loaded.Tunnel.ID)
	}
	if err := saveAt(configPath, secretPath, loaded); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{`"command"`, `"args"`, `"origin"`, `"public_url"`} {
		if strings.Contains(string(saved), obsolete) {
			t.Fatalf("obsolete generic tunnel field survived migration: %s", saved)
		}
	}
}

func TestDefaultServerUsesExposurePolicy(t *testing.T) {
	cfg := Default()
	if cfg.Server.Port != 37421 || cfg.Server.Expose {
		t.Fatalf("server = %#v", cfg.Server)
	}
}

func TestLegacyServerHostMigratesToExpose(t *testing.T) {
	for _, test := range []struct {
		name   string
		server string
		want   bool
	}{
		{name: "loopback", server: `{"host":"127.0.0.1","port":37421}`, want: false},
		{name: "localhost", server: `{"host":"localhost","port":37421}`, want: false},
		{name: "wildcard", server: `{"host":"0.0.0.0","port":37421}`, want: true},
		{name: "lan address", server: `{"host":"192.168.1.20","port":37421}`, want: true},
		{name: "explicit false wins", server: `{"host":"0.0.0.0","port":37421,"expose":false}`, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, "config.json")
			secretPath := filepath.Join(root, "tunnel.json")
			data := []byte(`{"server":` + test.server + `,"admin":{"enabled":false,"port":37422},"auth":{"mcp_enabled":false,"admin_enabled":false},"tunnel":{"enabled":false}}`)
			if err := os.WriteFile(configPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadAt(configPath, secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Server.Expose != test.want {
				t.Fatalf("expose = %t, want %t", loaded.Server.Expose, test.want)
			}
			if err := saveAt(configPath, secretPath, loaded); err != nil {
				t.Fatal(err)
			}
			saved, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(saved), `"host"`) {
				t.Fatalf("legacy host survived save: %s", saved)
			}
			wantExpose := `"expose": false`
			if test.want {
				wantExpose = `"expose": true`
			}
			if !strings.Contains(string(saved), wantExpose) {
				t.Fatalf("saved exposure missing: %s", saved)
			}
		})
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
