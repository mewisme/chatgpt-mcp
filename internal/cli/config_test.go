package cli

import (
	"os"
	"path/filepath"
	"reflect"
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
	if err := setConfigValue(&cfg, "admin.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "tunnel.args", `["a","b"]`); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4000 || cfg.Admin.Enabled || !reflect.DeepEqual(cfg.Tunnel.Args, []string{"a", "b"}) {
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
