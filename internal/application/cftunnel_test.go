package application

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func TestCFTunnelSnapshotDefaultDisabled(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	snap := CFTunnelSnapshot(cfg)
	if snap.PluginEnabled || snap.DesiredMCP || snap.DesiredAdmin {
		t.Fatalf("snapshot = %#v", snap)
	}
	if snap.MCP.AuthErr != nil || snap.Admin.AuthErr != nil {
		t.Fatalf("auth = %v %v", snap.MCP.AuthErr, snap.Admin.AuthErr)
	}
}

func TestCFTunnelSnapshotReadsEnableAndTarget(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	store.Builtins = pluginpkg.BuiltinRegistry{cftunnelplugin.Plugin()}
	if err := store.SetEnabled("cf-tunnel", true); err != nil {
		t.Fatal(err)
	}
	if err := (pluginpkg.SettingsStore{Layout: layout}).Set(cftunnelplugin.Plugin().Schema, "cf-tunnel", cftunnelplugin.TargetMCP, true); err != nil {
		t.Fatal(err)
	}
	snap := CFTunnelSnapshot(cfg)
	if !snap.PluginEnabled || !snap.DesiredMCP || snap.DesiredAdmin {
		t.Fatalf("snapshot = %#v", snap)
	}
}

func TestMCPExposureErrorRequiresAuth(t *testing.T) {
	cfg := config.Default()
	if err := MCPExposureError(cfg); err != cftunnelplugin.ErrMCPTokenMissing {
		t.Fatalf("error = %v", err)
	}
	cfg.Auth.MCPEnabled = false
	if err := MCPExposureError(cfg); err != cftunnelplugin.ErrMCPAuthDisabled {
		t.Fatalf("error = %v", err)
	}
	cfg.Server.Enabled = false
	if err := MCPExposureError(cfg); err != cftunnelplugin.ErrMCPHTTPDisabled {
		t.Fatalf("error = %v", err)
	}
}
