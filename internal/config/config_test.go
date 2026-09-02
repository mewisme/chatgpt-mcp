package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/secretstore"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
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

func TestValidateNetworkExposureRequiresAuth(t *testing.T) {
	for _, exposure := range []ExposureConfig{
		{Mode: ExposureAll, Interfaces: []string{}},
		{Mode: ExposureWildcard, Interfaces: []string{}},
		{Mode: ExposureInterfaces, Interfaces: []string{"eth0"}},
	} {
		t.Run(string(exposure.Mode), func(t *testing.T) {
			cfg := Default()
			cfg.Server.Expose = exposure
			cfg.Server.AllowInsecureHTTP = true
			cfg.Auth.MCPTokenHash = "mcp"
			cfg.Auth.AdminTokenHash = "admin"
			if err := Validate(cfg); err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name   string
				mutate func(*Config)
			}{
				{name: "mcp disabled", mutate: func(cfg *Config) { cfg.Auth.MCPEnabled = false }},
				{name: "admin auth disabled", mutate: func(cfg *Config) { cfg.Auth.AdminEnabled = false }},
				{name: "mcp token missing", mutate: func(cfg *Config) { cfg.Auth.MCPTokenHash = "" }},
				{name: "admin token missing", mutate: func(cfg *Config) { cfg.Auth.AdminTokenHash = "" }},
			} {
				t.Run(test.name, func(t *testing.T) {
					candidate := cfg
					test.mutate(&candidate)
					if err := Validate(candidate); err == nil {
						t.Fatal("network exposure accepted without required authentication")
					}
				})
			}
			cfg.Admin.Enabled = false
			cfg.Auth.AdminEnabled = false
			cfg.Auth.AdminTokenHash = ""
			if err := Validate(cfg); err != nil {
				t.Fatalf("disabled admin endpoint unnecessarily required admin auth: %v", err)
			}
		})
	}
}

func TestValidateNetworkExposureRequiresExplicitInsecureHTTPOptIn(t *testing.T) {
	cfg := Default()
	cfg.Server.Expose = ExposureConfig{Mode: ExposureAll, Interfaces: []string{}}
	cfg.Auth.MCPTokenHash = "mcp"
	cfg.Auth.AdminTokenHash = "admin"
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "allow_insecure_http") {
		t.Fatalf("network exposure without insecure HTTP opt-in = %v", err)
	}
	cfg.Server.AllowInsecureHTTP = true
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

func TestConfigSaveSeparatesTunnelSecrets(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.ID = "tunnel_0123456789abcdef0123456789abcdef"
	cfg.Tunnel.APIKey = "tunnel-secret"
	cfg.Tunnel.AdminKey = "admin-secret"
	cfg.Tunnel.AdminWorkspaceID = "ws-admin"

	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "tunnel-secret") || strings.Contains(string(configData), "admin-secret") || strings.Contains(string(configData), `"api_key"`) || strings.Contains(string(configData), `"admin_key"`) || strings.Contains(string(configData), "ws-admin") {
		t.Fatalf("config.json leaked tunnel secret: %s", configData)
	}
	secretData, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(secretData), "tunnel-secret") || strings.Contains(string(secretData), "admin-secret") || !strings.Contains(string(secretData), "ws-admin") || !strings.Contains(string(secretData), "runtime_key_configured") || !strings.Contains(string(secretData), "admin_key_configured") {
		t.Fatalf("tunnel.json did not contain marker-only secret metadata: %s", secretData)
	}
	loaded, err := loadAt(configPath, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tunnel.APIKey != "tunnel-secret" || loaded.Tunnel.AdminKey != "admin-secret" || loaded.Tunnel.AdminWorkspaceID != "ws-admin" || loaded.Tunnel.ID != cfg.Tunnel.ID {
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
			cfg.Tunnel.AdminKey = "admin-secret"
			cfg.Tunnel.AdminOrganizationID = "org-admin"
			if err := saveAt(configPath, secretPath, cfg); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadAt(configPath, secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Server.Port != cfg.Server.Port || loaded.Auth.MCPTokenHash != cfg.Auth.MCPTokenHash || loaded.Tunnel.APIKey != cfg.Tunnel.APIKey || loaded.Tunnel.AdminKey != cfg.Tunnel.AdminKey || loaded.Tunnel.AdminOrganizationID != cfg.Tunnel.AdminOrganizationID {
				t.Fatalf("round trip = %#v", loaded)
			}
			mainData, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(mainData), "tunnel-secret") || strings.Contains(string(mainData), "admin-secret") || strings.Contains(string(mainData), "org-admin") {
				t.Fatalf("main %s config leaked tunnel secret: %s", format, mainData)
			}
			secretData, err := os.ReadFile(secretPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(secretData), "tunnel-secret") || strings.Contains(string(secretData), "admin-secret") || !strings.Contains(string(secretData), "org-admin") {
				t.Fatalf("tunnel %s file leaked secret or lost scope: %s", format, secretData)
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
	if secret.APIKey != "" || !secret.RuntimeKeyConfigured {
		t.Fatalf("legacy tunnel file was not replaced by keyring marker: %#v", secret)
	}
	key, err := secretstore.New(root).Get(tunnelRuntimeSecretName)
	if err != nil || key != "legacy-secret" {
		t.Fatalf("migrated keyring secret = %q err=%v", key, err)
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
	if cfg.Server.Port != 37421 || cfg.Server.Expose.Mode != ExposureNone || len(cfg.Server.Expose.Interfaces) != 0 {
		t.Fatalf("server = %#v", cfg.Server)
	}
}

func TestDefaultFeaturesEnabled(t *testing.T) {
	cfg := Default()
	if !cfg.Features.Ponytail.Enabled || !cfg.Features.Caveman.Enabled {
		t.Fatalf("features = %#v", cfg.Features)
	}
}

func TestNormalizeShellPath(t *testing.T) {
	first := filepath.Join(t.TempDir(), "tools")
	second := filepath.Join(t.TempDir(), "bin")
	got, err := NormalizeShellPath([]string{first, second, first})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != filepath.Clean(first) || got[1] != filepath.Clean(second) {
		t.Fatalf("shell path = %#v", got)
	}
	if _, err := NormalizeShellPath([]string{"relative/bin"}); err == nil {
		t.Fatal("relative shell path was accepted")
	}
}

func TestLegacyConfigWithoutFeaturesKeepsEnabledDefaults(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			configPath := configformat.PathFor(root, "config", format)
			secretPath := configformat.PathFor(root, "tunnel", format)
			legacy := map[string]any{
				"server": map[string]any{"port": int64(37421), "expose": map[string]any{"mode": "none", "interfaces": []any{}}},
				"admin":  map[string]any{"enabled": false, "port": int64(37422)},
				"auth":   map[string]any{"mcp_enabled": false, "admin_enabled": false},
				"tunnel": map[string]any{"enabled": false},
			}
			data, err := configformat.EncodeGeneric(format, legacy)
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
			if !loaded.Features.Ponytail.Enabled || !loaded.Features.Caveman.Enabled {
				t.Fatalf("legacy %s features = %#v", format, loaded.Features)
			}
		})
	}
}

func TestPartialFeaturesKeepMissingFeatureDefault(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		t.Run(string(format), func(t *testing.T) {
			root := t.TempDir()
			configPath := configformat.PathFor(root, "config", format)
			secretPath := configformat.PathFor(root, "tunnel", format)
			partial := map[string]any{
				"server":   map[string]any{"port": int64(37421), "expose": map[string]any{"mode": "none", "interfaces": []any{}}},
				"admin":    map[string]any{"enabled": false, "port": int64(37422)},
				"auth":     map[string]any{"mcp_enabled": false, "admin_enabled": false},
				"features": map[string]any{"ponytail": map[string]any{"enabled": false}},
				"tunnel":   map[string]any{"enabled": false},
			}
			data, err := configformat.EncodeGeneric(format, partial)
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
			if loaded.Features.Ponytail.Enabled || !loaded.Features.Caveman.Enabled {
				t.Fatalf("partial %s features = %#v", format, loaded.Features)
			}
		})
	}
}

func TestLegacyServerHostMigratesToExpose(t *testing.T) {
	for _, test := range []struct {
		name   string
		server string
		want   ExposureMode
	}{
		{name: "loopback", server: `{"host":"127.0.0.1","port":37421}`, want: ExposureNone},
		{name: "localhost", server: `{"host":"localhost","port":37421}`, want: ExposureNone},
		{name: "wildcard", server: `{"host":"0.0.0.0","port":37421}`, want: ExposureWildcard},
		{name: "lan address", server: `{"host":"192.168.1.20","port":37421}`, want: ExposureAll},
		{name: "explicit false wins", server: `{"host":"0.0.0.0","port":37421,"expose":false}`, want: ExposureNone},
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
			if loaded.Server.Expose.Mode != test.want {
				t.Fatalf("expose = %#v, want %s", loaded.Server.Expose, test.want)
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
			if !strings.Contains(string(saved), `"expose": {`) || !strings.Contains(string(saved), `"mode": "`+string(test.want)+`"`) {
				t.Fatalf("saved exposure missing: %s", saved)
			}
		})
	}
}

func TestLegacyBooleanExposureMigratesAcrossFormats(t *testing.T) {
	for _, format := range []configformat.Format{configformat.JSON, configformat.YAML, configformat.TOML} {
		for _, test := range []struct {
			name  string
			value bool
			want  ExposureMode
		}{{"disabled", false, ExposureNone}, {"enabled", true, ExposureWildcard}} {
			t.Run(string(format)+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				path := configformat.PathFor(root, "config", format)
				secretPath := configformat.PathFor(root, "tunnel", format)
				legacy := map[string]any{
					"server": map[string]any{"port": int64(37421), "expose": test.value},
					"admin":  map[string]any{"enabled": false, "port": int64(37422)},
					"auth":   map[string]any{"mcp_enabled": false, "admin_enabled": false},
					"tunnel": map[string]any{"enabled": false},
				}
				data, err := configformat.EncodeGeneric(format, legacy)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				loaded, err := loadAt(path, secretPath)
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Server.Expose.Mode != test.want || len(loaded.Server.Expose.Interfaces) != 0 {
					t.Fatalf("expose = %#v", loaded.Server.Expose)
				}
			})
		}
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
	if _, err := secretstore.New(root).Get(tunnelRuntimeSecretName); !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("runtime key still exists in OS keyring: %v", err)
	}
}

func TestConfigSaveRollsBackMainConfigWhenTunnelSecretWriteFails(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	original := []byte(`{"server":{"port":4100},"admin":{"enabled":false,"port":4200},"auth":{"mcp_enabled":false,"admin_enabled":false},"tunnel":{"enabled":false}}`)
	if err := os.WriteFile(configPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte(`{"api_key":"old-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.APIKey = "new-secret"
	called := false
	if err := saveAtWithSecretSaver(configPath, secretPath, cfg, func(path string, value tunnel.Config) error {
		called = true
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			return readErr
		}
		if string(data) == string(original) {
			t.Fatal("main config was not written before secret persistence")
		}
		if writeErr := os.WriteFile(path, []byte(`{"api_key":"partial-new-secret"}`), 0600); writeErr != nil {
			return writeErr
		}
		return errors.New("injected tunnel secret write failure")
	}); err == nil {
		t.Fatal("expected tunnel secret write failure")
	}
	if !called {
		t.Fatal("secret saver was not called")
	}
	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != string(original) {
		t.Fatalf("main config was not rolled back:\n%s", saved)
	}
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != `{"api_key":"old-secret"}` {
		t.Fatalf("tunnel secret was not rolled back: %s", secret)
	}
}

func TestConfigSaveDoesNotTouchTunnelSecretWhenMainConfigWriteFails(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	if err := os.MkdirAll(filepath.Join(configPath, "block"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte(`{"api_key":"old-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Tunnel.APIKey = "new-secret"
	if err := saveAt(configPath, secretPath, cfg); err == nil {
		t.Fatal("expected main config write failure")
	}
	saved, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != `{"api_key":"old-secret"}` {
		t.Fatalf("tunnel secret changed after main config failure: %s", saved)
	}
}

func TestClearingRuntimeKeyPreservesAdminKeySecret(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	secretPath := filepath.Join(root, "tunnel.json")
	cfg := Default()
	cfg.Tunnel.APIKey = "runtime-secret"
	cfg.Tunnel.AdminKey = "admin-secret"
	cfg.Tunnel.AdminWorkspaceID = "ws_admin"
	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Tunnel.APIKey = ""
	if err := saveAt(configPath, secretPath, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAt(configPath, secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tunnel.APIKey != "" || loaded.Tunnel.AdminKey != "admin-secret" || loaded.Tunnel.AdminWorkspaceID != "ws_admin" {
		t.Fatalf("loaded tunnel = %#v", loaded.Tunnel)
	}
}
