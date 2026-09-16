package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/notification"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestReloadTunnelCollectionKeepsUnchangedClients(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	instances := []tunnel.InstanceConfig{{ID: "a", APIKey: "key-a"}, {ID: "b", APIKey: "key-b"}}
	admins := []tunnel.AdminConfig{}
	cfg.Tunnel.Instances, cfg.Tunnel.Admins = &instances, &admins
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	clientA, _ := a.Tunnels.Client("a")
	clientB, _ := a.Tunnels.Client("b")
	next := cfg
	changed := []tunnel.InstanceConfig{{ID: "a", APIKey: "key-a"}, {ID: "b", APIKey: "new-key"}, {ID: "c", APIKey: "key-c"}}
	next.Tunnel.Instances = &changed
	if err := a.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Tunnels.Client("a"); got != clientA {
		t.Fatal("unchanged tunnel a restarted")
	}
	if got, _ := a.Tunnels.Client("b"); got == clientB {
		t.Fatal("changed tunnel b was not replaced")
	}
	if _, ok := a.Tunnels.Client("c"); !ok {
		t.Fatal("added tunnel c missing")
	}
	removed := []tunnel.InstanceConfig{{ID: "a", APIKey: "key-a"}}
	next.Tunnel.Instances = &removed
	if err := a.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Tunnels.Client("a"); got != clientA {
		t.Fatal("removing another tunnel restarted a")
	}
	if _, ok := a.Tunnels.Client("b"); ok {
		t.Fatal("removed tunnel b still attached")
	}
}

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

func TestReloadConfigUpdatesShellPath(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	next := cfg
	next.Shell.Path = []string{t.TempDir()}
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	if got := app.Config.Snapshot().Shell.Path; len(got) != 1 || got[0] != next.Shell.Path[0] {
		t.Fatalf("stored shell path = %#v", got)
	}
	if got := app.Tools.Workspaces.ShellPath(); len(got) != 1 || got[0] != next.Shell.Path[0] {
		t.Fatalf("runtime shell path = %#v", got)
	}
}

func TestReloadConfigUpdatesShellExecutable(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	bash := filepath.Join(t.TempDir(), "bash")
	if err := os.WriteFile(bash, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	next := cfg
	next.Shell.Executable = bash
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	provider, err := app.Tools.Shell.Provider()
	if err != nil {
		t.Fatal(err)
	}
	if app.Config.Snapshot().Shell.Executable != bash || provider.Source != "configured" || filepath.Clean(provider.Executable) != filepath.Clean(bash) {
		t.Fatalf("shell config=%q provider=%#v", app.Config.Snapshot().Shell.Executable, provider)
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
	if err := app.Tools.Registry.ReplaceOwned("plugin:ponytail", nil); err != nil {
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
	next.Shell.Path = []string{t.TempDir()}
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
	if len(got.Shell.Path) != len(previous.Shell.Path) {
		t.Fatalf("committed shell path not restored: %#v", got.Shell.Path)
	}
	if runtimePath := app.Tools.Workspaces.ShellPath(); len(runtimePath) != len(previous.Shell.Path) {
		t.Fatalf("runtime shell path = %#v, want %#v", runtimePath, previous.Shell.Path)
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
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	reloadTestAfterCommit = func() {
		close(entered)
		<-release
	}
	defer func() { reloadTestAfterCommit = nil }()
	next := cfg
	next.Shell.Path = []string{t.TempDir()}
	errCh := make(chan error, 1)
	go func() { errCh <- app.ReloadConfig(next) }()
	<-entered
	if got := app.Config.Snapshot().Shell.Path; len(got) != 1 || got[0] != next.Shell.Path[0] {
		t.Fatalf("store after commit = %#v", got)
	}
	if got := app.Tools.Workspaces.ShellPath(); len(got) != 0 {
		t.Fatalf("runtime changed before apply: %#v", got)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if got := app.Tools.Workspaces.ShellPath(); len(got) != 1 || got[0] != next.Shell.Path[0] {
		t.Fatalf("runtime after apply = %#v", got)
	}
}

type recordingNotifier struct {
	sent chan notification.Notification
}

func (r *recordingNotifier) Available(context.Context) bool { return true }
func (r *recordingNotifier) Capabilities(context.Context) notification.Capabilities {
	return notification.Capabilities{Notification: true}
}
func (r *recordingNotifier) Send(_ context.Context, note notification.Notification) error {
	select {
	case r.sent <- note:
	default:
	}
	return nil
}

func TestReloadConfigAppliesNotificationSettings(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app.Notifications.Stop()
	provider := &recordingNotifier{sent: make(chan notification.Notification, 4)}
	app.Notifications = notification.New(notification.Options{
		Provider: provider,
		Presence: func() (bool, error) { return false, nil },
		Settings: func() notification.Settings { return app.Config.Snapshot().Notifications },
	})
	app.Notifications.Start(app.Tools.Approvals.Events())
	publishApproval(t, app.Tools.Approvals)
	select {
	case <-provider.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification before disable")
	}
	next := app.Config.Snapshot()
	next.Notifications.Enabled = false
	if err := app.ReloadConfig(next); err != nil {
		t.Fatal(err)
	}
	publishApproval(t, app.Tools.Approvals)
	select {
	case note := <-provider.sent:
		t.Fatalf("notified after disable: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}

func publishApproval(t *testing.T, manager *approval.Manager) {
	t.Helper()
	challenge, _, err := manager.CreateChallenge(approval.ChallengeInput{
		SessionID: "session", WorkspaceID: "ws_test", Source: "tunnel", TargetTool: "run_command",
		Arguments: map[string]any{"command": "true"}, GuardCode: controlguard.CodeControlPlaneMutation,
		GuardReason: "guarded", Title: "Allow test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.CreateRequest(challenge.ID, "session", "ws_test"); err != nil {
		t.Fatal(err)
	}
}
