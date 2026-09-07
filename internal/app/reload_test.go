package app

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

func TestReloadConfigUpdatesLiveRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	app := New(cfg)
	next := cfg
	next.Auth.MCPEnabled = true
	next.Auth.MCPTokenHash = "hash"
	next.Features.Ponytail.Active = false
	next.Permissions.AllowDirs = []string{t.TempDir()}
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	got := app.Config.Snapshot()
	if !got.Auth.MCPEnabled || got.Features.Ponytail.Active || len(got.Permissions.AllowDirs) != 1 {
		t.Fatalf("runtime config = %#v", got)
	}
	if _, ok := app.Tools.Registry.Schema("ponytail_turn"); !ok {
		t.Fatal("inactive feature controller tool disappeared")
	}
}

func TestReloadConfigUpdatesShellApprovalPolicy(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	app := New(cfg)
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "balanced" {
		t.Fatalf("initial shell approval policy = %q", got)
	}
	next := cfg
	next.Shell.ApprovalPolicy = "strict"
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got := app.Config.Snapshot().Shell.ApprovalPolicy; got != "strict" {
		t.Fatalf("stored shell approval policy = %q", got)
	}
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "strict" {
		t.Fatalf("runtime shell approval policy = %q", got)
	}
}

func TestReloadConfigSyncsTunnelAdminKeyWithoutRuntimeReconfigure(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	app := New(cfg)
	next := cfg
	next.Tunnel.AdminKey = "admin-key"
	next.Tunnel.AdminWorkspaceID = "ws_admin"
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got := app.Tunnel.Config(); got.AdminKey != "admin-key" || got.AdminWorkspaceID != "ws_admin" {
		t.Fatalf("tunnel config = %#v", got)
	}
	if !app.Tunnel.Status().AdminKeyConfigured {
		t.Fatalf("tunnel status = %#v", app.Tunnel.Status())
	}
}
