package application

import (
	"context"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

func TestConfiguredTunnelProviderMissingWhenUninstalled(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	if _, err := ConfiguredTunnelProviderStatus(config.Default(), "cf"); err == nil {
		t.Fatal("expected missing provider")
	}
	if got := ListConfiguredTunnelProviders(config.Default()); len(got) != 0 {
		t.Fatalf("providers = %#v", got)
	}
}

func TestConfiguredTunnelProviderReadsEnableAndTarget(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	testutil.InstallStubTunnelPlugin(t, true)
	if err := (pluginpkg.SettingsStore{Layout: pluginpkg.DefaultLayout()}).Set(testutil.TunnelProviderSettingsSchema(), "cf-tunnel", "mcp", true); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	item, err := ConfiguredTunnelProviderStatus(cfg, "cf")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Enabled || !targetDesired(item, "mcp") || targetDesired(item, "admin") {
		t.Fatalf("status = %#v", item)
	}
}

func TestMCPExposureErrorRequiresAuth(t *testing.T) {
	cfg := config.Default()
	if err := MCPExposureError(cfg); err != tunnelprovider.ErrMCPTokenMissing {
		t.Fatalf("error = %v", err)
	}
	cfg.Auth.MCPEnabled = false
	if err := MCPExposureError(cfg); err != tunnelprovider.ErrMCPAuthDisabled {
		t.Fatalf("error = %v", err)
	}
	cfg.Server.Enabled = false
	if err := MCPExposureError(cfg); err != tunnelprovider.ErrMCPHTTPDisabled {
		t.Fatalf("error = %v", err)
	}
}

func TestStartCFTunnelAllFailsBeforePersist(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	testutil.InstallStubTunnelPlugin(t, false)
	cfg := config.Default()
	if err := StartCFTunnel(context.Background(), cfg, "all"); err != tunnelprovider.ErrMCPTokenMissing {
		t.Fatalf("error = %v", err)
	}
	item, err := ConfiguredTunnelProviderStatus(cfg, "cf")
	if err != nil {
		t.Fatal(err)
	}
	if item.Enabled {
		t.Fatal("start all persisted plugin enable")
	}
}

func TestStartCFTunnelMCPPersistsDesired(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	testutil.InstallStubTunnelPlugin(t, false)
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	if err := StartCFTunnel(context.Background(), cfg, "mcp"); err != nil {
		t.Fatal(err)
	}
	item, err := ConfiguredTunnelProviderStatus(cfg, "cf")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Enabled || !targetDesired(item, "mcp") || targetDesired(item, "admin") {
		t.Fatalf("status = %#v", item)
	}
	if err := StopCFTunnel(context.Background(), "mcp"); err != nil {
		t.Fatal(err)
	}
	item, err = ConfiguredTunnelProviderStatus(cfg, "cf")
	if err != nil {
		t.Fatal(err)
	}
	if targetDesired(item, "mcp") {
		t.Fatalf("stop left mcp desired: %#v", item)
	}
}

func TestStartCFTunnelRejectsUnknownTarget(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	testutil.InstallStubTunnelPlugin(t, false)
	if err := StartCFTunnel(context.Background(), config.Default(), "ssh"); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestLookupUsesInstalledPluginNotBuiltin(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	if _, err := LookupTunnelProvider("cf"); err == nil {
		t.Fatal("expected missing provider without install")
	}
	testutil.InstallStubTunnelPlugin(t, false)
	ref, err := LookupTunnelProvider("cf")
	if err != nil {
		t.Fatal(err)
	}
	if ref.PluginID != "cf-tunnel" || ref.Path == "" {
		t.Fatalf("ref = %#v", ref)
	}
}

func targetDesired(item runtimecontrol.TunnelProviderStatus, target string) bool {
	for _, candidate := range item.Targets {
		if candidate.Target == target {
			return candidate.Desired
		}
	}
	return false
}
