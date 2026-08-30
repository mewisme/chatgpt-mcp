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
	next.Features.Ponytail.Enabled = false
	next.Permissions.AllowDirs = []string{t.TempDir()}
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	got := app.Config.Snapshot()
	if !got.Auth.MCPEnabled || got.Features.Ponytail.Enabled || len(got.Permissions.AllowDirs) != 1 {
		t.Fatalf("runtime config = %#v", got)
	}
	if _, ok := app.Tools.Registry.Schema("ponytail_turn"); ok {
		t.Fatal("disabled feature tool remained registered")
	}
}
