package config

import (
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestFieldSetValuePreservesTypedBehaviorAndLegacyAliases(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	for key, value := range map[string]string{
		"server.port": "4000", "server.expose": "true", "admin.enabled": "false",
		"features.ponytail.enabled": "false", "features.ponytail.mode": "ULTRA", "features.caveman.enabled": "false", "features.caveman.mode": "WENYAN-ULTRA",
		"permissions.allow_dirs": "/tmp\n/var/tmp", "shell.path": "/opt/tools,/usr/local/custom/bin", "shell.approval_policy": "DENY",
		"shell.approval_allow_commands": "git status\ngo test *\ngit status", "shell.approval_deny_commands": "git push *",
		"shell.environment_policy": "FILTERED", "shell.environment_allow": "DATABASE_URL, CUSTOM_VALUE, database_url", "shell.network_policy": "DENY",
	} {
		if err := SetValue(&cfg, key, value); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if cfg.Server.Port != 4000 || cfg.Server.Expose.Mode != ExposureWildcard || cfg.Admin.Enabled || cfg.Features.Ponytail.Active || cfg.Features.Ponytail.Mode != "ultra" || cfg.Features.Caveman.Active || cfg.Features.Caveman.Mode != "wenyan-ultra" || len(cfg.Permissions.AllowDirs) != 2 || len(cfg.Shell.Path) != 2 || cfg.Shell.ApprovalPolicy != "deny" || len(cfg.Shell.ApprovalAllowCommands) != 2 || len(cfg.Shell.ApprovalDenyCommands) != 1 || cfg.Shell.EnvironmentPolicy != "filtered" || len(cfg.Shell.EnvironmentAllow) != 2 || cfg.Shell.NetworkPolicy != "deny" {
		t.Fatalf("cfg=%#v", cfg)
	}
	if value, err := RawValue(cfg, "shell.approval_policy"); err != nil || value != "deny" {
		t.Fatalf("shell approval policy value=%q err=%v", value, err)
	}
	if value, err := RawValue(cfg, "shell.approval_allow_commands"); err != nil || value != "git status,go test *" {
		t.Fatalf("shell approval allow commands=%q err=%v", value, err)
	}
	if value, err := RawValue(cfg, "shell.approval_deny_commands"); err != nil || value != "git push *" {
		t.Fatalf("shell approval deny commands=%q err=%v", value, err)
	}
	if value, err := RawValue(cfg, "shell.environment_policy"); err != nil || value != "filtered" {
		t.Fatalf("shell environment policy value=%q err=%v", value, err)
	}
	if value, err := RawValue(cfg, "shell.environment_allow"); err != nil || value != "CUSTOM_VALUE,DATABASE_URL" {
		t.Fatalf("shell environment allow value=%q err=%v", value, err)
	}
	if value, err := RawValue(cfg, "shell.network_policy"); err != nil || value != "deny" {
		t.Fatalf("shell network policy value=%q err=%v", value, err)
	}
}

func TestInteractiveFieldIsRemoved(t *testing.T) {
	if _, ok := FieldByKey("interactive"); ok {
		t.Fatal("interactive field still exposed")
	}
	cfg := Default()
	if err := SetValue(&cfg, "interactive", "false"); err == nil || !strings.Contains(err.Error(), "unsupported config key") {
		t.Fatalf("err=%v", err)
	}
}

func TestFieldSetValueValidationIsTransactional(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPTokenHash = "mcp"
	cfg.Auth.AdminTokenHash = "admin"
	original := cfg.Server.Port
	if err := SetValueValidated(&cfg, "server.port", "70000"); err == nil || !strings.Contains(err.Error(), "between 1 and 65535") {
		t.Fatalf("err=%v", err)
	}
	if cfg.Server.Port != original {
		t.Fatalf("invalid value mutated config: %d", cfg.Server.Port)
	}
	if err := SetValueValidated(&cfg, "server.enabled", "false"); err == nil || !strings.Contains(err.Error(), "at least one MCP transport") {
		t.Fatalf("last MCP transport disable err=%v", err)
	}
	if !cfg.Server.Enabled {
		t.Fatal("invalid transport update mutated config")
	}
	if err := SetValue(&cfg, "features.ponytail.mode", "review"); err == nil || err.Error() != "features.ponytail.mode must be lite, full, or ultra" {
		t.Fatalf("ponytail err=%v", err)
	}
	if err := SetValue(&cfg, "features.caveman.mode", "wenyan"); err == nil || !strings.Contains(err.Error(), "wenyan-lite") {
		t.Fatalf("caveman err=%v", err)
	}
}

func TestFieldReadOnlyAndSensitiveValuesNeverExposeSecrets(t *testing.T) {
	cfg := Default()
	cfg.Auth.MCPTokenHash = "mcp-secret"
	cfg.Auth.AdminTokenHash = "admin-secret"
	cfg.Tunnel.APIKey = "runtime-secret"
	cfg.Tunnel.AdminKey = "admin-tunnel-secret"
	for _, key := range []string{"auth.mcp_token_hash", "auth.admin_token_hash", "tunnel.api_key", "tunnel.admin_key"} {
		spec, ok := FieldByKey(key)
		if !ok || !spec.Sensitive {
			t.Fatalf("spec=%#v ok=%t", spec, ok)
		}
		value, err := DisplayValue(cfg, spec)
		if err != nil || value != "configured" {
			t.Fatalf("%s display=%q err=%v", key, value, err)
		}
	}
	tree, err := RedactedTree(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(strings.TrimSpace(toJSONForTest(t, tree)))
	for _, secret := range []string{"mcp-secret", "admin-secret", "runtime-secret", "admin-tunnel-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
	if value, err := RedactedValueAt(cfg, "auth.mcp_token_hash"); err != nil || value != RedactedValue {
		t.Fatalf("redacted value=%#v err=%v", value, err)
	}
}

func TestFieldsReturnsDefensiveCopy(t *testing.T) {
	fields := Fields()
	if len(fields) == 0 {
		t.Fatal("no fields")
	}
	fields[0].Key = "mutated"
	for index := range fields {
		if len(fields[index].Options) > 0 {
			fields[index].Options[0] = "mutated"
			break
		}
	}
	next := Fields()
	if next[0].Key == "mutated" {
		t.Fatal("field key mutation escaped copy")
	}
	for _, spec := range next {
		for _, option := range spec.Options {
			if option == "mutated" {
				t.Fatal("field option mutation escaped copy")
			}
		}
	}
}

func toJSONForTest(t *testing.T, value any) string {
	t.Helper()
	data, err := configformat.Marshal(configformat.JSON, value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
