package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestInitializeAndAuthLifecycle(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	result, err := Initialize(InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigPath != filepath.Join(root, "config.json") || result.Format != configformat.JSON {
		t.Fatalf("init result = %#v", result)
	}
	if !strings.HasPrefix(result.MCPToken, "mcp_") || !strings.HasPrefix(result.AdminToken, "admin_") {
		t.Fatalf("generated tokens have unexpected format")
	}
	status, err := GetAuthStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.MCPEnabled || !status.MCPConfigured || !status.AdminEnabled || !status.AdminConfigured {
		t.Fatalf("status = %#v", status)
	}
	status, err = SetAuthEnabled("mcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.MCPEnabled || !status.MCPConfigured {
		t.Fatalf("disabled status = %#v", status)
	}
	rotated, status, err := RotateAuthToken("mcp")
	if err != nil {
		t.Fatal(err)
	}
	if rotated == result.MCPToken || !status.MCPEnabled || !status.MCPConfigured {
		t.Fatalf("rotation did not replace and enable MCP auth")
	}
	if _, _, err := RotateAuthToken("missing"); err == nil {
		t.Fatal("invalid auth kind unexpectedly accepted")
	}
}

func TestInitializePreservesFormatOnForce(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	first, err := Initialize(InitOptions{Format: configformat.YAML, FormatSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Format != configformat.YAML {
		t.Fatalf("format = %s", first.Format)
	}
	if _, err := Initialize(InitOptions{}); err == nil {
		t.Fatal("existing config unexpectedly overwritten without force")
	}
	forced, err := Initialize(InitOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if forced.Format != configformat.YAML || filepath.Ext(forced.ConfigPath) != ".yaml" {
		t.Fatalf("forced result = %#v", forced)
	}
	if _, err := Initialize(InitOptions{Force: true, Format: configformat.TOML, FormatSelected: true}); err == nil {
		t.Fatal("init force unexpectedly changed storage format")
	}
}

func TestUninitializeRemovesManagedRoot(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(InitOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Uninitialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("config root still exists: %v", err)
	}
	if err := RemoveConfigRoot(t.TempDir()); err == nil {
		t.Fatal("unmanaged root unexpectedly removed")
	}
}

func TestSetAuthEnabledRequiresConfiguredToken(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := SetAuthEnabled("mcp", true); err == nil {
		t.Fatal("MCP auth enabled without token")
	}
	if _, err := SetAuthEnabled("admin", true); err == nil {
		t.Fatal("admin auth enabled without token")
	}
}
