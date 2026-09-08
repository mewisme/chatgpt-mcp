package application

import (
	"context"
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
	status, err = SetAuthEnabled(t.Context(), "mcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.MCPEnabled || !status.MCPConfigured {
		t.Fatalf("disabled status = %#v", status)
	}
	rotated, status, err := RotateAuthToken(t.Context(), "mcp")
	if err != nil {
		t.Fatal(err)
	}
	if rotated == result.MCPToken || !status.MCPEnabled || !status.MCPConfigured {
		t.Fatalf("rotation did not replace and enable MCP auth")
	}
	if _, _, err := RotateAuthToken(t.Context(), "missing"); err == nil {
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
	if _, err := SetAuthEnabled(t.Context(), "mcp", true); err == nil {
		t.Fatal("MCP auth enabled without token")
	}
	if _, err := SetAuthEnabled(t.Context(), "admin", true); err == nil {
		t.Fatal("admin auth enabled without token")
	}
}

func TestConfigMutationUsesDomainValidationAndPreservesSecrets(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	cfg.Tunnel.APIKey = "runtime-secret"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	want := cfg
	wantErr := config.SetValueValidated(&want, "server.port", "70000")
	if wantErr == nil {
		t.Fatal("domain validation unexpectedly accepted invalid port")
	}
	if _, err := SetConfigField(t.Context(), "server.port", "70000"); err == nil || err.Error() != wantErr.Error() {
		t.Fatalf("application validation err=%v want=%v", err, wantErr)
	}
	result, err := SetConfigField(t.Context(), "server.port", "40123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Server.Port != 40123 || result.Config.Auth.MCPTokenHash != "mcp-hash" || result.Config.Auth.AdminTokenHash != "admin-hash" || result.Config.Tunnel.APIKey != "runtime-secret" {
		t.Fatalf("config mutation changed unrelated values: %#v", result.Config)
	}
}

func TestConfigConvertRoundTripJSONYAMLTOML(t *testing.T) {
	defer configformat.SetRootPath("")
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	cfg.Server.Port = 40123
	cfg.Features.Ponytail.Mode = "ultra"
	if err := config.SaveAs(cfg, configformat.JSON); err != nil {
		t.Fatal(err)
	}
	for _, format := range []configformat.Format{configformat.YAML, configformat.TOML, configformat.JSON} {
		if _, err := ConvertConfig(format); err != nil {
			t.Fatalf("convert to %s: %v", format, err)
		}
		verified, err := VerifyConfig()
		if err != nil {
			t.Fatalf("verify %s: %v", format, err)
		}
		if verified.Format != format {
			t.Fatalf("verified format=%s want=%s", verified.Format, format)
		}
		loaded, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Server.Port != 40123 || loaded.Features.Ponytail.Mode != "ultra" || loaded.Auth.MCPTokenHash != "mcp-hash" || loaded.Auth.AdminTokenHash != "admin-hash" {
			t.Fatalf("round trip changed config after %s: %#v", format, loaded)
		}
	}
}

func TestConfigExportImportPreservesSafetyAndState(t *testing.T) {
	defer configformat.SetRootPath("")
	base := t.TempDir()
	root := filepath.Join(base, "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	cfg.Server.Port = 40123
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(base, "backup.cgm")
	if _, err := ExportConfig(bundle, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportConfig(bundle, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("export overwrite safety err=%v", err)
	}
	if _, err := SetConfigField(t.Context(), "server.port", "40234"); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportConfig(context.Background(), bundle, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("import replacement safety err=%v", err)
	}
	if _, err := ImportConfig(context.Background(), bundle, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != 40123 || loaded.Auth.MCPTokenHash != "mcp-hash" || loaded.Auth.AdminTokenHash != "admin-hash" {
		t.Fatalf("import did not restore original config: %#v", loaded)
	}
}

func TestRuntimeRunningWithoutControlState(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	running, err := RuntimeRunning(context.Background())
	if err != nil || running {
		t.Fatalf("running=%t err=%v", running, err)
	}
}
