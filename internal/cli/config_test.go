package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
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
	if err := setConfigValue(&cfg, "features.ponytail.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "features.caveman.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := setConfigValue(&cfg, "permissions.allow_dirs", "/tmp,/var/tmp"); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4000 || cfg.Server.Expose.Mode != config.ExposureWildcard || cfg.Admin.Enabled || cfg.Features.Ponytail.Enabled || cfg.Features.Caveman.Enabled || cfg.Tunnel.ControlPlaneBaseURL != "https://api.openai.com" || cfg.Tunnel.OrganizationID != "org-test" || len(cfg.Permissions.AllowDirs) != 2 {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestFeatureConfigTraversal(t *testing.T) {
	cfg := config.Default()
	value, err := getConfigValue(cfg, "features")
	if err != nil {
		t.Fatal(err)
	}
	features, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("features = %#v", value)
	}
	ponytail, ok := features["ponytail"].(map[string]any)
	if !ok || ponytail["enabled"] != true {
		t.Fatalf("ponytail = %#v", features["ponytail"])
	}
	leaf, err := getConfigValue(cfg, "features.caveman.enabled")
	if err != nil || leaf != true {
		t.Fatalf("caveman leaf = %#v %v", leaf, err)
	}
}

func TestSensitiveConfigValuesAreRedacted(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "secret"
	for _, key := range []string{"auth.mcp_token_hash", "auth.admin_token_hash", "tunnel.api_key"} {
		value, err := getConfigValue(cfg, key)
		if err != nil {
			t.Fatal(err)
		}
		if value != redactedValue {
			t.Fatalf("%s = %#v, want %q", key, value, redactedValue)
		}
	}
}

func TestConfigParentTraversalAndFlatOutput(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "tunnel-secret"

	parent, err := getConfigValue(cfg, "admin")
	if err != nil {
		t.Fatal(err)
	}
	object, ok := parent.(map[string]any)
	if !ok || object["enabled"] != true || object["port"] != int64(37422) {
		t.Fatalf("admin subtree = %#v", parent)
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := printConfigSelection(cmd, cfg, "admin", true, configOutputOptions{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"admin.enabled = true", "admin.port = 37422"} {
		if !strings.Contains(text, want) {
			t.Fatalf("flat output missing %q:\n%s", want, text)
		}
	}
}

func TestConfigOutputFormatsAndRedaction(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "tunnel-secret"

	for _, test := range []struct {
		name    string
		options configOutputOptions
		want    []string
	}{
		{name: "json", options: configOutputOptions{json: true}, want: []string{`"auth"`, `"mcp_token_hash": "<redacted>"`}},
		{name: "yaml", options: configOutputOptions{yaml: true}, want: []string{"auth:", `mcp_token_hash: <redacted>`}},
		{name: "toml", options: configOutputOptions{toml: true}, want: []string{"[auth]", `mcp_token_hash = '<redacted>'`}},
		{name: "format yaml", options: configOutputOptions{format: "yaml"}, want: []string{"auth:", `admin_token_hash: <redacted>`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := printConfigSelection(cmd, cfg, "", true, test.options); err != nil {
				t.Fatal(err)
			}
			text := out.String()
			if strings.Contains(text, "mcp-secret") || strings.Contains(text, "admin-secret") || strings.Contains(text, "tunnel-secret") {
				t.Fatalf("secret leaked in %s output: %s", test.name, text)
			}
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Fatalf("%s output missing %q:\n%s", test.name, want, text)
				}
			}
		})
	}
}

func TestConfigOutputFormatConflict(t *testing.T) {
	if _, _, err := resolveConfigOutputFormat(configOutputOptions{format: "json", yaml: true}); err == nil {
		t.Fatal("expected conflicting output formats to fail")
	}
	format, selected, err := resolveConfigOutputFormat(configOutputOptions{format: "json", json: true})
	if err != nil || !selected || format != configformat.JSON {
		t.Fatalf("same format flags = %q selected=%t err=%v", format, selected, err)
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
	if cfg.Server.Expose.Mode != config.ExposureAll || cfg.Admin.Enabled {
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

func TestConfigCommandAliases(t *testing.T) {
	cmd := configCommand()
	convert, _, err := cmd.Find([]string{"transform"})
	if err != nil || convert.Name() != "convert" {
		t.Fatalf("transform alias = %v %v", convert, err)
	}
	verify, _, err := cmd.Find([]string{"validate"})
	if err != nil || verify.Name() != "verify" {
		t.Fatalf("validate alias = %v %v", verify, err)
	}
}

func TestConfigHasNoAllowDirSubcommand(t *testing.T) {
	for _, command := range configCommand().Commands() {
		if command.Name() == "allow-dir" {
			t.Fatal("config allow-dir should not exist")
		}
	}
}

func TestRemoveConfigRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "chatgpt-mcp")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := configformat.MarkRoot(root); err != nil {
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
