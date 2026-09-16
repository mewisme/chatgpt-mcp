package application

import (
	"context"
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

func TestStartCFTunnelAllFailsBeforePersist(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	if err := StartCFTunnel(context.Background(), cfg, "all"); err != cftunnelplugin.ErrMCPTokenMissing {
		t.Fatalf("error = %v", err)
	}
	if CFTunnelSnapshot(cfg).PluginEnabled {
		t.Fatal("start all persisted plugin enable")
	}
}

func TestStartCFTunnelMCPPersistsDesired(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	if err := StartCFTunnel(context.Background(), cfg, "mcp"); err != nil {
		t.Fatal(err)
	}
	snap := CFTunnelSnapshot(cfg)
	if !snap.PluginEnabled || !snap.DesiredMCP || snap.DesiredAdmin {
		t.Fatalf("snapshot = %#v", snap)
	}
	if err := StopCFTunnel(context.Background(), "mcp"); err != nil {
		t.Fatal(err)
	}
	snap = CFTunnelSnapshot(cfg)
	if snap.DesiredMCP {
		t.Fatalf("stop left mcp desired: %#v", snap)
	}
}

func TestStartCFTunnelRejectsUnknownTarget(t *testing.T) {
	if err := StartCFTunnel(context.Background(), config.Default(), "ssh"); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestCFTunnelStatusFromSnapshotDefaultNil(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	if got := CFTunnelStatusFromSnapshot(CFTunnelSnapshot(config.Default())); got != nil {
		t.Fatalf("status = %#v", got)
	}
}
