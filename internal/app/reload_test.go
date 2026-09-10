package app

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestReloadConfigUpdatesLiveRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
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
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "balanced" {
		t.Fatalf("initial shell approval policy = %q", got)
	}
	next := cfg
	next.Shell.ApprovalPolicy = "strict"
	next.Shell.EnvironmentPolicy = "filtered"
	next.Shell.EnvironmentAllow = []string{"DATABASE_URL"}
	next.Shell.Path = []string{t.TempDir()}
	next.Shell.SandboxPolicy = "required"
	next.Shell.NetworkPolicy = "deny"
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got := app.Config.Snapshot().Shell.ApprovalPolicy; got != "strict" {
		t.Fatalf("stored shell approval policy = %q", got)
	}
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "strict" {
		t.Fatalf("runtime shell approval policy = %q", got)
	}
	if got := app.Tools.Workspaces.ShellEnvironmentPolicy(); got != "filtered" {
		t.Fatalf("runtime shell environment policy = %q", got)
	}
	if got := app.Tools.Workspaces.ShellEnvironmentAllow(); len(got) != 1 || got[0] != "DATABASE_URL" {
		t.Fatalf("runtime shell environment allow = %#v", got)
	}
	if got := app.Tools.Workspaces.ShellPath(); len(got) != 1 || got[0] != next.Shell.Path[0] {
		t.Fatalf("runtime shell path = %#v", got)
	}
	if got := app.Tools.Workspaces.ShellSandboxPolicy(); got != "required" {
		t.Fatalf("runtime shell sandbox policy = %q", got)
	}
	if got := app.Tools.Workspaces.ShellNetworkPolicy(); got != "deny" {
		t.Fatalf("runtime shell network policy = %q", got)
	}
}

func TestReloadConfigSyncsTunnelAdminKeyWithoutRuntimeReconfigure(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
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

func TestReloadConfigFailedApplyRestoresCommittedConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	previous := app.Config.Snapshot()

	if err := app.Tools.Registry.ReplaceOwned("feature:ponytail", nil); err != nil {
		t.Fatal(err)
	}
	if err := app.Tools.Registry.Register("ponytail_turn", tools.Schema{Name: "ponytail_turn"}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("blocked"), nil
	}); err != nil {
		t.Fatal(err)
	}

	next := previous
	next.Auth.MCPEnabled = true
	next.Auth.MCPTokenHash = "hash"
	next.Features.Ponytail.Active = !previous.Features.Ponytail.Active
	next.Permissions.AllowDirs = []string{t.TempDir()}
	next.Shell.ApprovalPolicy = "strict"
	if err := app.ReloadConfig(next); err == nil {
		t.Fatal("expected apply failure")
	}

	got := app.Config.Snapshot()
	if got.Auth.MCPEnabled != previous.Auth.MCPEnabled || got.Auth.MCPTokenHash != previous.Auth.MCPTokenHash {
		t.Fatalf("committed auth not restored: %#v", got.Auth)
	}
	if got.Features.Ponytail.Active != previous.Features.Ponytail.Active {
		t.Fatalf("committed features not restored: %#v", got.Features)
	}
	if len(got.Permissions.AllowDirs) != len(previous.Permissions.AllowDirs) {
		t.Fatalf("committed permissions not restored: %#v", got.Permissions)
	}
	if got.Shell.ApprovalPolicy != previous.Shell.ApprovalPolicy {
		t.Fatalf("committed shell policy not restored: %q", got.Shell.ApprovalPolicy)
	}
	if policy := app.Tools.Workspaces.ShellApprovalPolicy(); string(policy) != previous.Shell.ApprovalPolicy {
		t.Fatalf("runtime shell policy = %q, want %q", policy, previous.Shell.ApprovalPolicy)
	}
	if app.Tools.Features().Ponytail.Active != previous.Features.Ponytail.Active {
		t.Fatalf("runtime features = %#v", app.Tools.Features())
	}
}

func TestReloadConfigCommitsBeforeRuntimeApply(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	cfg.Shell.ApprovalPolicy = "balanced"
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var badPairs atomic.Uint64
	var wg sync.WaitGroup
	reloadTestAfterCommit = func() {
		close(entered)
		<-release
	}
	defer func() { reloadTestAfterCommit = nil }()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-entered
			for {
				select {
				case <-release:
					return
				default:
				}
				store := app.Config.Snapshot().Shell.ApprovalPolicy
				runtime := string(app.Tools.Workspaces.ShellApprovalPolicy())
				if store != "strict" {
					badPairs.Add(1)
				}
				if runtime == "strict" && store == "balanced" {
					badPairs.Add(1)
				}
			}
		}()
	}

	next := cfg
	next.Shell.ApprovalPolicy = "strict"
	errCh := make(chan error, 1)
	go func() { errCh <- app.ReloadConfig(next) }()

	<-entered
	if got := app.Config.Snapshot().Shell.ApprovalPolicy; got != "strict" {
		t.Fatalf("store after commit = %q, want strict", got)
	}
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "balanced" {
		t.Fatalf("runtime before apply = %q, want balanced", got)
	}
	close(release)
	wg.Wait()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if badPairs.Load() != 0 {
		t.Fatalf("observers saw %d inconsistent mid-reload samples", badPairs.Load())
	}
	if got := app.Tools.Workspaces.ShellApprovalPolicy(); got != "strict" {
		t.Fatalf("runtime after apply = %q", got)
	}
}
